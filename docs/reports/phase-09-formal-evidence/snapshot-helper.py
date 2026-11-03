"""阶段9正式批次的只读快照；默认plan，显式--execute才查询固定新医生库。"""
from pathlib import Path
from decimal import Decimal, InvalidOperation
import argparse
import datetime as dt
import hashlib
import json
import math
import os
import re
import subprocess
import sys

RUN = 'phase09-formal-20261103T181204Z-terra'
CASES = ['OT01','OT02','OT03','OT04','OT05']
MODEL, JUDGE = 'gpt-6.1-terra', 'deepseek-flash'
ACCOUNTS = {'phase-09-openai':'USD','phase-09-deepseek':'CNY'}
ROOT = Path.cwd()
TEMP = Path(os.environ['TEMP'])/'opsdoctor-phase09'
MAX_RAW, MAX_NS = 64<<20, 300_000_000_000

class SnapshotError(Exception):
    pass

def require(ok, reason):
    if not ok: raise SnapshotError(reason)

def utc():
    return dt.datetime.now(dt.timezone.utc).isoformat().replace('+00:00','Z')

def digest(raw):
    return hashlib.sha256(raw).hexdigest()

def canonical(value):
    return json.dumps(value,ensure_ascii=False,sort_keys=True,separators=(',',':'))

def integer(value):
    return isinstance(value,int) and not isinstance(value,bool)

def stamp(value):
    try:
        parsed = dt.datetime.fromisoformat(value.replace('Z','+00:00'))
        require(parsed.tzinfo is not None, 'timestamp has no timezone')
        return parsed
    except (ValueError,AttributeError,TypeError):
        raise SnapshotError('UTC timestamp missing/invalid') from None

def duration(value,prefix='',capped=True):
    # 不从UTC或数据库时间差补造单调耗时。
    known=value.get(prefix+'monotonic_elapsed_known') is True
    ns,seconds=value.get(prefix+'elapsed_ns'),value.get(prefix+'elapsed_seconds')
    valid=False
    if known and integer(ns) and ns>=0:
        try:
            number=Decimal(str(seconds))
            valid=number.is_finite() and number>=0 and abs(number*1_000_000_000-ns)<=1
        except (InvalidOperation,TypeError,ValueError): pass
    return {'known':known,'consistent':bool(valid),'elapsed_ns':ns if valid else None,
            'elapsed_seconds':seconds if valid else None,
            'within_300_seconds':bool(valid and ns<=MAX_NS) if capped else None}

def validate_run(value):
    require(bool(re.fullmatch(r'phase09-formal-[A-Za-z0-9_-]{1,49}',value)) and value==RUN,
            'only the authorized fixed formal run is allowed; reject Mock/pilot/other IDs')
    return value

class Evidence:
    def __init__(self,path):
        path.mkdir(parents=True,exist_ok=False)
        self.path,self.records=path,[]
    def save(self,label,raw,origin):
        require(len(raw)<=MAX_RAW,'evidence exceeds64MiB: '+label)
        target=self.path/f'{len(self.records)+1:03d}-{label}.log'
        with target.open('xb') as stream:
            stream.write(raw); stream.flush(); os.fsync(stream.fileno())
        require(target.read_bytes()==raw,'copy differs: '+label)
        item={'path':str(target),'origin':origin,'bytes':len(raw),'sha256':digest(raw)}
        self.records.append(item)
        return item
    def read(self,path,label):
        raw=path.read_bytes(); self.save(label,raw,str(path)); return raw
    def sql(self,label,statement):
        require(statement in queries(RUN).values() and statement.startswith('SELECT ')
                and statement.count(';')==1 and statement.endswith(';'),'only fixed single SELECTs allowed')
        # 仅使用固定新MySQL容器的既有环境认证，不读取/打印环境或私有文件。
        args=['docker','exec','-i','opsdoctor-phase09-doctor-mysql','sh','-c',
              'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql --default-character-set=utf8mb4 --batch --raw --skip-column-names -uroot doctor']
        response=subprocess.run(args,cwd=ROOT,input=statement.encode('utf-8'),
                                stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=45)
        self.save(label+'-stdout',response.stdout,'fixed new doctor DB UTF8 SELECT')
        self.save(label+'-stderr',response.stderr,'fixed new doctor DB stderr')
        require(response.returncode==0,'read-only SELECT failed: '+label)
        return [json.loads(line) for line in response.stdout.decode('utf-8').splitlines() if line.strip()]

def queries(run):
    validate_run(run)
    batch=("SELECT JSON_OBJECT('id',id,'target',target,'config',config,'status',status,"
           "'token_limit',token_limit,'tokens_used',tokens_used,'stopped_reason',stopped_reason,"
           "'created_at',created_at,'updated_at',updated_at) FROM eval_runs WHERE id='"+run+"';")
    return {
      'batch_before':batch,
      'trials':"SELECT JSON_OBJECT('run_id',run_id,'scenario',scenario,'iteration',iteration,'state',state,"
        "'fault_started_at',fault_started_at,'restored_at',restored_at,'updated_at',updated_at)"
        f" FROM eval_trials WHERE run_id='{run}' ORDER BY scenario,iteration;",
      'slots':"SELECT JSON_OBJECT('result_key',r.result_key,'run_id',r.run_id,'scenario',r.scenario,"
        "'iteration',r.iteration,'group',r.experiment_group,'profile',r.execution_profile,"
        "'position',r.position,'attempt',r.attempt,'retry_of',r.retry_of,'input',r.input,"
        "'diagnosis_id',r.diagnosis_id,'judge_id',r.judge_id,'result',r.result,'judgement',r.judgement,"
        "'doctor',JSON_OBJECT('status',d.status,'model',d.model,'provider',d.provider,"
        "'report_mode',d.report_mode,'error',d.error,'budget_reason',d.budget_reason,'report',d.report),"
        "'judge',JSON_OBJECT('status',j.status,'model',j.model,'provider',j.provider,'error',j.error))"
        " FROM eval_results r LEFT JOIN diagnoses d ON d.id=r.diagnosis_id LEFT JOIN diagnoses j ON j.id=r.judge_id"
        f" WHERE r.run_id='{run}' ORDER BY r.scenario,r.iteration,r.position,r.attempt;",
      # eval_attempts为权威桥；OR捕获异常无桥费用，显式报错而非静默漏记。
      'attempts':"SELECT JSON_OBJECT('id',a.id,'diagnosis_id',a.diagnosis_id,'account',a.account,"
        "'currency',b.currency,'diagnosis_currency',d.currency,'diagnosis_run_id',d.eval_run_id,"
        "'linked_run_id',ea.run_id,'trigger',d.`trigger`,'requested_model',a.requested_model,"
        "'maximum_micros',a.maximum_micros,'charged_micros',a.charged_micros,'status',a.status,"
        "'tokens_in',a.tokens_in,'tokens_out',a.tokens_out,'cache_hit_tokens',a.cache_hit_tokens,"
        "'cache_write_tokens',a.cache_write_tokens,'reasoning_tokens',a.reasoning_tokens,"
        "'maximum_tokens',ea.maximum_tokens,'charged_tokens',ea.charged_tokens,"
        "'created_at',a.created_at,'settled_at',a.settled_at)"
        " FROM llm_attempts a JOIN diagnoses d ON d.id=a.diagnosis_id"
        " JOIN llm_budget b ON b.account=a.account LEFT JOIN eval_attempts ea ON ea.attempt_id=a.id"
        f" WHERE ea.run_id='{run}' OR d.eval_run_id='{run}' ORDER BY a.created_at,a.id;",
      'diagnoses':"SELECT JSON_OBJECT('id',id,'trigger',`trigger`,'eval_run_id',eval_run_id,'eval_key',eval_key,"
        "'status',status,'model',model,'provider',provider,'currency',currency,'eval_account',eval_account,"
        "'cost_micros',cost_micros,'tokens_in',tokens_in,'tokens_out',tokens_out,'error',error)"
        f" FROM diagnoses WHERE eval_run_id='{run}' ORDER BY id;",
      'accounts':"SELECT JSON_OBJECT('account',account,'currency',currency,'spent_micros',spent_micros,"
        "'stopped_reason',stopped_reason) FROM llm_budget"
        " WHERE account IN ('phase-09-openai','phase-09-deepseek') ORDER BY account;",
      'interruptions':"SELECT JSON_OBJECT('id',id,'run_id',run_id,'scenario',scenario,'iteration',iteration,"
        "'state',state,'reason',reason,'created_at',created_at)"
        f" FROM eval_interruptions WHERE run_id='{run}' ORDER BY id;",
      'batch_after':batch}

def collection_quality(summary):
    audit=summary.get('collector_audit')
    if summary.get('data_quality')=='affected': return 'affected'
    if not isinstance(audit,dict): return 'unknown'
    counts=[audit.get(k) for k in ('dropping_data_events','log_dropped_items','mapper_error_lines','mapper_error_occurrences')]
    for name in ('signal_dropped_items','signal_dropping_data_events'):
        value=audit.get(name)
        if not isinstance(value,dict): return 'unknown'
        counts+=list(value.values())
    if any(integer(n) and n>0 for n in counts): return 'affected'
    if not all(integer(n) and n>=0 for n in counts): return 'unknown'
    try:
        good=(summary.get('data_quality')=='normal' and audit.get('status')=='ok' and not audit.get('error')
          and integer(audit.get('raw_bytes')) and audit['raw_bytes']>0 and bool(audit.get('raw_path'))
          and bool(audit.get('container_id')) and re.fullmatch(r'[a-f0-9]{64}',str(audit.get('sha256','')))
          and stamp(audit.get('window_end'))>stamp(audit.get('window_start')))
        return 'normal' if good else 'unknown'
    except SnapshotError: return 'unknown'

def verify_collector(row,evidence,collector_root):
    summary=row.get('result') or {}; audit=summary.get('collector_audit') or {}
    result={'saved_quality':collection_quality(summary),'chain_verified':False,'errors':[]}
    try:
        prefix=digest(row['result_key'].encode('utf-8'))
        expected=collector_root.resolve()/(prefix+'.log')
        require(Path(audit.get('raw_path','')).resolve()==expected,'Collector raw path not bound to slot')
        raw=evidence.read(expected,prefix+'-collector-raw')
        require(len(raw)==audit.get('raw_bytes') and digest(raw)==audit.get('sha256'),'Collector raw SHA/bytes differ')
        originals={}
        for suffix in ('start','audit','summary'):
            originals[suffix]=json.loads(evidence.read(expected.with_suffix('.'+suffix+'.json'),prefix+'-'+suffix))
        require(canonical(originals['audit'])==canonical(audit),'Collector audit differs from DB')
        require(canonical(originals['summary'])==canonical(summary),'Collector summary differs from DB')
        start=originals['start']; identity=start.get('identity') or {}
        require(start.get('key')==row['result_key'] and start.get('start')==audit.get('window_start')
          and identity.get('id')==audit.get('container_id') and identity.get('running') is True
          and integer(identity.get('restarts')) and identity['restarts']>=0
          and stamp(identity.get('started'))<stamp(start.get('start')),'Collector start identity/window differs')
        anchor=start.get('anchor','')
        require(isinstance(anchor,str) and ' ' in anchor and stamp(anchor.split(' ',1)[0])<=stamp(start['start']),
                'Collector anchor missing/after start')
        result.update(chain_verified=True,raw_sha256=digest(raw),raw_bytes=len(raw))
    except (SnapshotError,OSError,KeyError,TypeError,ValueError) as error:
        result['errors'].append(str(error))
    result['verified_quality']=result['saved_quality']
    if result['saved_quality']=='normal' and not result['chain_verified']: result['verified_quality']='unknown'
    return result

def costs(snapshot):
    totals={c:{'charged_micros':0,'attempts':0,'not_ok_or_unsettled_micros':0,
               'unsettled_reserved_micros':0,'statuses':{}} for c in ('USD','CNY')}
    problems,seen,token_total=[],set(),0
    for attempt in snapshot.get('attempts',[]):
        if attempt.get('id') in seen:
            problems.append('duplicate attempt ID'); continue
        seen.add(attempt.get('id'))
        if attempt.get('linked_run_id')!=RUN or attempt.get('diagnosis_run_id')!=RUN:
            problems.append('attempt run bridge mismatch: '+str(attempt.get('id')))
        currency,charge=attempt.get('currency'),attempt.get('charged_micros')
        if (currency not in totals or ACCOUNTS.get(attempt.get('account'))!=currency
          or attempt.get('diagnosis_currency')!=currency or not integer(charge) or charge<0):
            problems.append('attempt account/currency/charge invalid: '+str(attempt.get('id'))); continue
        entry=totals[currency]; entry['charged_micros']+=charge; entry['attempts']+=1
        status=str(attempt.get('status')); entry['statuses'][status]=entry['statuses'].get(status,0)+1
        if status!='ok' or not attempt.get('settled_at'): entry['not_ok_or_unsettled_micros']+=charge
        if not attempt.get('settled_at'): entry['unsettled_reserved_micros']+=charge
        tokens=attempt.get('charged_tokens')
        if integer(tokens) and tokens>=0: token_total+=tokens
        else: problems.append('attempt charged_tokens missing/invalid: '+str(attempt.get('id')))
    projects=[]
    for account in snapshot.get('accounts',[]):
        item=dict(account); spent=item.get('spent_micros')
        if (item.get('account') not in ACCOUNTS or item.get('currency')!=ACCOUNTS.get(item.get('account'))
          or not integer(spent) or spent<0): problems.append('project account invalid')
        else: item.update(authorized_cap_micros=10_000_000,remaining_to_authorized_cap_micros=10_000_000-spent,
                          cap_source='user authorization; not persisted database limit')
        projects.append(item)
    if {v.get('account') for v in projects}!=set(ACCOUNTS) or len(projects)!=2:
        problems.append('expected two phase09 currency accounts')
    return {'run_all_attempts_by_currency':totals,'cost_binding_verified':not problems,'project_accounts_cumulative':projects,
            'charged_tokens_including_unknown_reservations':token_total,'issues':problems,
            'note':'All statuses include failures, exclusions and unknown/unsettled reservations. If bridge/currency issues exist, totals are candidate amounts awaiting reconciliation, not verified run costs. Never add currencies.'}

def summarize(snapshot,evidence,collector_root):
    batches=snapshot.get('batch_before',[])
    require(len(batches)==1,'fixed formal run missing/ambiguous')
    batch=batches[0]; require(batch.get('id')==RUN and batch.get('target')=='otel-demo','batch identity/target differs')
    config=batch.get('config') or {}; require(isinstance(config,dict),'frozen config is not object')
    issues,lookup=[],{}
    if canonical(snapshot.get('batch_after'))!=canonical(batches):
        issues.append('batch changed during sequential SELECTs; not a stable final snapshot')
    for row in snapshot.get('slots',[]):
        key=(row.get('scenario'),row.get('iteration'),row.get('group'))
        expected=RUN+'/'+str(key[0])+'/1/primary-'+str(key[2])
        if (key[0] not in CASES or key[1]!=1 or key[2] not in ('E1','E2') or row.get('run_id')!=RUN
          or row.get('profile')!='primary' or row.get('result_key')!=expected or row.get('attempt')!=0 or row.get('retry_of')):
            issues.append('unexpected slot retained without inclusion: '+str(row.get('result_key'))); continue
        require(key not in lookup,'duplicate formal slot'); lookup[key]=row
    groups={g:{'planned_slots':5,'observed_slots':0,'normal_reports':0,'component_matches':0,'component_n':0,
               'valid_scores':0,'judge_failed_missing':0,'judge_not_saved_missing':0,'judge_excluded_missing':0,
               'mechanism':{'correct':0,'partial':0,'incorrect':0},'coverage_n':0,'coverage_mean':None,
               'coverage_values':[],'excluded':{}} for g in ('E1','E2')}
    detail=[]
    for scenario in CASES:
        for group in ('E1','E2'):
            stats=groups[group]; row=lookup.get((scenario,1,group))
            item={'scenario':scenario,'iteration':1,'group':group,'observed':row is not None,
                  'component_match':None,'mechanism':None,'coverage':None}
            if row is None:
                item.update(classification='not_created',judge_state='not_started'); detail.append(item); continue
            stats['observed_slots']+=1; item['result_key']=row['result_key']
            summary,grade=row.get('result'),row.get('judgement')
            if not isinstance(summary,dict) or not summary:
                classification='not_completed'; item['doctor_status']=(row.get('doctor') or {}).get('status')
            else:
                collector=verify_collector(row,evidence,collector_root); item['collector']=collector
                item['doctor_status'],item['report_mode']=summary.get('status'),summary.get('report_mode')
                if collector['verified_quality']!='normal': classification='collection_'+collector['verified_quality']
                elif summary.get('mixed_model') is not False: classification='mixed_or_unverified_model'
                elif summary.get('reliability_failure') is not False: classification='reliability_failure'
                elif (summary.get('model')!=MODEL or summary.get('actual_models')!=[MODEL]
                  or (row.get('doctor') or {}).get('model')!=MODEL or (row.get('doctor') or {}).get('provider')!='openai'):
                    classification='other_or_unverified_model'
                elif summary.get('status')!='succeeded' or not summary.get('report') or not isinstance(summary.get('component_match'),bool):
                    classification='doctor_failed_or_no_report'
                else: classification='normal'
            item['classification']=classification
            item['judge_state']=grade.get('status') if isinstance(grade,dict) else 'not_saved'
            if classification!='normal':
                stats['excluded'][classification]=stats['excluded'].get(classification,0)+1; detail.append(item); continue
            stats['normal_reports']+=1; stats['component_n']+=1
            item['component_match']=summary['component_match']; stats['component_matches']+=int(summary['component_match'])
            if not isinstance(grade,dict) or not grade: stats['judge_not_saved_missing']+=1
            elif grade.get('status')=='failed': stats['judge_failed_missing']+=1
            else:
                mechanism=(grade.get('grade') or {}).get('mechanism'); coverage=grade.get('coverage')
                good=(grade.get('status')=='ok' and grade.get('model')==JUDGE and grade.get('version')=='rubric-v1.1'
                  and bool(config.get('rubric_hash')) and grade.get('hash')==config['rubric_hash']
                  and mechanism in stats['mechanism'] and isinstance(coverage,(int,float)) and not isinstance(coverage,bool)
                  and math.isfinite(coverage) and 0<=coverage<=1)
                if good:
                    stats['valid_scores']+=1; stats['mechanism'][mechanism]+=1; stats['coverage_values'].append(coverage)
                    item['mechanism'],item['coverage']=mechanism,coverage
                else: stats['judge_excluded_missing']+=1
            detail.append(item)
    for stats in groups.values():
        values=stats.pop('coverage_values'); stats['coverage_n']=len(values)
        stats['coverage_mean']=math.fsum(values)/len(values) if values else None
    fee=costs(snapshot); issues.extend(fee['issues'])
    if batch.get('tokens_used')!=fee['charged_tokens_including_unknown_reservations']:
        issues.append('tokens_used differs from sum original eval_attempts charged_tokens')
    interruptions=snapshot.get('interruptions',[])
    trials=snapshot.get('trials',[])
    trial_keys=[(v.get('scenario'),v.get('iteration')) for v in trials]
    if len(set(trial_keys))!=len(trial_keys) or any(s not in CASES or n!=1 for s,n in trial_keys):
        issues.append('unexpected/duplicate trials retained')
    return {'run_status':batch.get('status'),'stopped_reason':batch.get('stopped_reason'),
      'interrupted':batch.get('status')=='interrupted' or bool(interruptions),'interruptions':interruptions,
      'expected_trials':5,'expected_slots':10,'observed_trials':len(trials),'observed_slot_rows':len(snapshot.get('slots',[])),
      'trials':trials,'slots':detail,'terra_primary_groups':groups,'costs':fee,'integrity_issues':issues,
      'denominator_note':'Normal primary Terra reports enter component_n; saved ok grades only enter mechanism/coverage. Empty slots and failed/missing grades are missing, never zero. No Mock/pilot/other runs queried.'}

def local_observations(evidence,recovery_root,config):
    result={'baseline':[],'scenario_evidence':[],'missing':[],'errors':[],
      'note':'Original monotonic measurements only, never DB/UTC differences. Missing files can be expected for interrupted runs.'}
    if not recovery_root.is_dir(): result['missing'].append(str(recovery_root)); return result
    for path in sorted(recovery_root.glob('baseline-*.json')):
        try:
            value=json.loads(evidence.read(path,path.stem))
            require(value.get('run_id')==RUN and value.get('kind')=='baseline','baseline identity differs')
            result['baseline'].append({'path':str(path),'original':value,'whole_call_duration':duration(value,capped=False),
              'health_wait_duration':duration(value,'health_'),'complete_passed_rounds':value.get('status')=='passed'
              and integer(value.get('polls')) and value['polls']>=1 and value.get('queries')==5*value['polls']
              and value.get('scenarios')==CASES})
        except (OSError,ValueError,TypeError,SnapshotError) as error: result['errors'].append(str(error))
    for scenario in CASES:
        path=recovery_root/(scenario+'-1.json'); item={'scenario':scenario,'iteration':1}
        if path.is_file():
            try:
                raw=evidence.read(path,scenario+'-original-flags'); original=json.loads(raw)
                require(original.get('run_id')==RUN and original.get('scenario')==scenario and original.get('iteration')==1,
                        'recovery original identity differs')
                item['original_record']=original; proof_path=Path(str(path)+'.restored')
                if proof_path.is_file():
                    proof=json.loads(evidence.read(proof_path,scenario+'-restored-proof')); text=raw.decode('utf-8')
                    match=re.search(r'"original"\s*:\s*',text); require(match is not None,'original RawMessage missing')
                    _,end=json.JSONDecoder().raw_decode(text,match.end())
                    item['restored_proof']=proof
                    stamp(proof.get('confirmed_at'))
                    item['original_flags_and_restore_hash_verified']=(proof.get('original_sha256')==digest(text[match.end():end].encode())
                      and canonical(original.get('original'))==canonical(config.get('baseline_flags')))
                else: result['missing'].append(str(proof_path))
            except (OSError,ValueError,TypeError,SnapshotError) as error: result['errors'].append(str(error))
        else: result['missing'].append(str(path))
        for kind in ('symptom','recovery'):
            observed_path=recovery_root/(scenario+'-1.'+kind+'-observed.json')
            if not observed_path.is_file(): result['missing'].append(str(observed_path)); continue
            try:
                value=json.loads(evidence.read(observed_path,scenario+'-'+kind+'-timing'))
                require(value.get('run_id')==RUN and value.get('scenario')==scenario and value.get('iteration')==1
                  and value.get('kind')==kind,'observation identity differs')
                item[kind]={'path':str(observed_path),'original':value,'duration':duration(value)}
            except (OSError,ValueError,TypeError,SnapshotError) as error: result['errors'].append(str(error))
        result['scenario_evidence'].append(item)
    return result

def capture(opts,evidence):
    snapshot={}
    for label,statement in queries(opts.run).items(): snapshot[label]=evidence.sql(label,statement)
    config=snapshot['batch_before'][0].get('config',{}) if len(snapshot['batch_before'])==1 else {}
    result={'snapshot':snapshot,'analysis':summarize(snapshot,evidence,TEMP/'collector-audit')}
    result['local_observations']=local_observations(evidence,TEMP/'recovery'/opts.run,config)
    status=result['analysis']['run_status']
    result['run_state_accepted']=status=='completed' or status=='interrupted' and opts.accept_interrupted
    result['accept_interrupted']=opts.accept_interrupted
    result['snapshot_integrity_verified']=not result['analysis']['integrity_issues']
    result['capture_note']='capture_succeeded only means data was retrieved; it is not an approval or data-integrity pass.'
    return result

def self_test():
    # 全部假数据在内存构造，不访问实际DB、Docker、HTTP、模型或本次原件。
    import copy
    class FakeEvidence:
        def __init__(self,data): self.data=data
        def read(self,path,label):
            if str(path) not in self.data: raise FileNotFoundError(str(path))
            return self.data[str(path)]
    collector_root=TEMP/'formal-snapshot-selftest-fake-collector'
    originals,slots={},[]
    for scenario in CASES:
        for group in ('E1','E2'):
            key=RUN+'/'+scenario+'/1/primary-'+group
            path=collector_root/(digest(key.encode())+'.log')
            raw=b'2026-11-03T18:12:01Z synthetic preserved Collector line\n'
            audit={'window_start':'2026-11-03T18:12:00Z','window_end':'2026-11-03T18:12:02Z','status':'ok',
              'container_id':'a'*64,'raw_path':str(path),'sha256':digest(raw),'raw_bytes':len(raw),
              'dropping_data_events':0,'log_dropped_items':0,'mapper_error_lines':0,'mapper_error_occurrences':0,
              'signal_dropped_items':{},'signal_dropping_data_events':{}}
            summary={'status':'succeeded','model':MODEL,'actual_models':[MODEL],'mixed_model':False,
              'reliability_failure':False,'report':{'synthetic':True},'component_match':True,
              'data_quality':'normal','collector_audit':audit,'report_mode':'voluntary'}
            start={'key':key,'start':audit['window_start'],'identity':{'id':'a'*64,'started':'2026-11-03T18:00:00Z',
              'running':True,'restarts':0},'anchor':'2026-11-03T18:11:59Z synthetic anchor'}
            originals[str(path)]=raw
            for suffix,value in (('start',start),('audit',audit),('summary',summary)):
                originals[str(path.with_suffix('.'+suffix+'.json'))]=canonical(value).encode()
            slots.append({'result_key':key,'run_id':RUN,'scenario':scenario,'iteration':1,'group':group,
              'profile':'primary','attempt':0,'retry_of':None,'result':summary,'doctor':{'model':MODEL,'provider':'openai'},
              'judgement':{'status':'ok','model':JUDGE,'version':'rubric-v1.1','hash':'frozen',
                          'grade':{'mechanism':'correct'},'coverage':0.75}})
    batch={'id':RUN,'target':'otel-demo','status':'completed','tokens_used':30,'config':{'rubric_hash':'frozen'}}
    attempts=[{'id':'x','account':'phase-09-openai','currency':'USD','diagnosis_currency':'USD',
      'linked_run_id':RUN,'diagnosis_run_id':RUN,'charged_micros':17,'charged_tokens':10,
      'status':'network_error','settled_at':None},{'id':'y','account':'phase-09-deepseek','currency':'CNY',
      'diagnosis_currency':'CNY','linked_run_id':RUN,'diagnosis_run_id':RUN,'charged_micros':23,'charged_tokens':20,
      'status':'ok','settled_at':'synthetic'}]
    base={'batch_before':[batch],'batch_after':[batch],'slots':slots,
      'trials':[{'scenario':c,'iteration':1} for c in CASES],'attempts':attempts,'interruptions':[],
      'accounts':[{'account':a,'currency':c,'spent_micros':100} for a,c in ACCOUNTS.items()]}
    checks=[]
    def run(value): return summarize(value,FakeEvidence(originals),collector_root)
    good=run(base); assert not good['integrity_issues']
    assert good['terra_primary_groups']['E1']['normal_reports']==5
    assert good['terra_primary_groups']['E2']['coverage_mean']==0.75
    checks.append('complete10 normal reports and per-group exact denominator')
    value=copy.deepcopy(base)
    value['slots'][0]['judgement']={'status':'failed','coverage':0,'grade':{'mechanism':'incorrect'}}
    result=run(value)['terra_primary_groups']['E1']
    assert result['component_n']==5 and result['valid_scores']==4 and result['judge_failed_missing']==1
    assert result['mechanism']['incorrect']==0 and result['coverage_mean']==0.75
    checks.append('failed grade remains missing, never zero')
    value=copy.deepcopy(base)
    value['slots'][0]['judgement']['grade']['mechanism']='partial'
    value['slots'][2]['judgement']['grade']['mechanism']='incorrect'
    result=run(value)['terra_primary_groups']['E1']
    assert result['mechanism']=={'correct':3,'partial':1,'incorrect':1} and result['coverage_n']==5
    checks.append('all three frozen mechanism categories counted separately')
    value=copy.deepcopy(base); value['slots'][0]['judgement']=None
    result=run(value)['terra_primary_groups']['E1']
    assert result['component_n']==5 and result['valid_scores']==4 and result['judge_not_saved_missing']==1
    checks.append('unsaved grade remains missing with report denominator intact')
    value=copy.deepcopy(base); value['slots']=value['slots'][2:]
    result=run(value)
    assert len(result['slots'])==10 and result['terra_primary_groups']['E1']['component_n']==4
    assert result['slots'][0]['classification']=='not_created' and result['slots'][0]['mechanism'] is None
    checks.append('interrupted missing slots retained outside denominators')
    value=copy.deepcopy(base); value['slots'][0]['result']=None
    assert run(value)['slots'][0]['classification']=='not_completed'
    checks.append('empty result is not a scored failure')
    for field,replacement,classification in (('data_quality','affected','collection_affected'),
      ('mixed_model',True,'mixed_or_unverified_model'),('reliability_failure',True,'reliability_failure')):
        value=copy.deepcopy(base); value['slots'][0]['result'][field]=replacement
        row=value['slots'][0]; path=Path(row['result']['collector_audit']['raw_path']).with_suffix('.summary.json')
        old=originals[str(path)]; originals[str(path)]=canonical(row['result']).encode()
        result=run(value); assert result['slots'][0]['classification']==classification
        assert result['costs']['run_all_attempts_by_currency']['USD']['charged_micros']==17
        originals[str(path)]=old; checks.append(classification+' excluded, all costs retained')
    path=Path(base['slots'][0]['result']['collector_audit']['raw_path'])
    old=originals[str(path)]; originals[str(path)]=b'tampered'
    assert run(base)['slots'][0]['classification']=='collection_unknown'; originals[str(path)]=old
    checks.append('Collector hash mismatch cannot remain normal')
    fees=good['costs']['run_all_attempts_by_currency']
    assert fees['USD']['charged_micros']==17 and fees['CNY']['charged_micros']==23
    assert fees['USD']['unsettled_reserved_micros']==17
    checks.append('failed and unsettled costs retained separately by currency')
    value=copy.deepcopy(base); value['attempts'][0]['linked_run_id']='other'
    assert run(value)['integrity_issues']; checks.append('attempt bridge inconsistency surfaced')
    for bad in ('phase09-dualmock-x','phase09-pilot2-20261103-terra',RUN+"' OR 1=1"):
        try: validate_run(bad)
        except SnapshotError: pass
        else: raise AssertionError('unsafe run accepted')
    checks.append('Mock/pilot/injected run IDs rejected')
    assert all(q.startswith('SELECT ') and q.count(';')==1 for q in queries(RUN).values())
    checks.append('eight fixed single SELECT statements')
    known={'monotonic_elapsed_known':True,'elapsed_ns':300_000_000_000,'elapsed_seconds':300}
    assert duration(known)['within_300_seconds'] is True
    known['elapsed_ns']+=1; known['elapsed_seconds']=300.000000001
    assert duration(known)['within_300_seconds'] is False
    assert duration({'start_utc':'2026-01-01T00:00:00Z','passed_utc':'2026-01-01T00:00:01Z'})['elapsed_ns'] is None
    checks.append('300s boundary and missing monotonic time explicit')
    return {'self_test':'passed','checks':checks,'services_accessed':False,'actual_run_originals_read':False}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run',default=RUN)
    parser.add_argument('--execute',action='store_true',help='explicit fixed SELECTs and local original reads only')
    parser.add_argument('--accept-interrupted',action='store_true',help='accept interrupted run; retain empty slots as missing')
    parser.add_argument('--output',help='new JSON below TEMP/opsdoctor-phase09/formal-snapshots only')
    parser.add_argument('--self-test',action='store_true',help='in-memory fake data only; no services or actual originals')
    opts=parser.parse_args(); validate_run(opts.run)
    require(not(opts.execute and opts.self_test),'self-test cannot execute services')
    if opts.self_test:
        print(json.dumps(self_test(),ensure_ascii=False,indent=2)); return 0
    if not opts.execute:
        print(json.dumps({'mode':'plan','run_id':opts.run,'services_accessed':False,'statements':queries(opts.run),
          'accept_interrupted':opts.accept_interrupted,
          'local_original_roots':[str(TEMP/'collector-audit'),str(TEMP/'recovery'/opts.run)],
          'note':'Only --execute reads fixed new DB and originals; no login, Key, HTTP, model, faults or SQL writes.'},ensure_ascii=False,indent=2))
        return 0
    require((ROOT/'AGENTS.md').is_file(),'execute from repository root')
    output_root=(TEMP/'formal-snapshots').resolve()
    output=Path(opts.output).resolve() if opts.output else output_root/(opts.run+'-'+dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')+'.json')
    require(output.is_relative_to(output_root) and output.suffix=='.json','output must be new JSON inside fixed TEMP formal-snapshots')
    output.parent.mkdir(parents=True,exist_ok=True)
    # 只独占创建快照。失败时也保留已取得原件，不覆盖任何既存证据。
    with output.open('xb') as stream:
        evidence=Evidence(Path(str(output)+'.evidence'))
        result={'kind':'phase09_formal_readonly_snapshot','checked_at_utc':utc(),'run_id':opts.run,
          'helper_sha256':digest(Path(__file__).read_bytes()),'capture_succeeded':False,
          'currency_note':'USD按项目用量账本计；Key与其他用途共用，平台账单无法单独核对。USD与CNY分开。',
          'actions':'fixed SELECT and local byte reads only; no regrading or database writes'}
        try:
            result.update(capture(opts,evidence)); result['capture_succeeded']=True
        except Exception as error:
            result['capture_error']=type(error).__name__+': '+str(error)
        result['evidence']=evidence.records; result['finished_at_utc']=utc()
        stream.write((json.dumps(result,ensure_ascii=False,indent=2,allow_nan=False)+'\n').encode())
        stream.flush(); os.fsync(stream.fileno())
    print(json.dumps({'output':str(output),'sha256':digest(output.read_bytes()),
      'capture_succeeded':result['capture_succeeded'],'run_state_accepted':result.get('run_state_accepted'),
      'run_status':result.get('analysis',{}).get('run_status')},ensure_ascii=False))
    return 0 if result['capture_succeeded'] and result.get('run_state_accepted') else 2

if __name__=='__main__':
    try: sys.exit(main())
    except SnapshotError as error:
        print(json.dumps({'error':str(error)},ensure_ascii=False),file=sys.stderr); sys.exit(2)
