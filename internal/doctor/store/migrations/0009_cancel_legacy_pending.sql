-- 阶段 5 只保存、不执行。升级时这些旧告警不能变成一批付费诊断。
UPDATE diagnoses SET status='cancelled',error='告警已过时，未执行',finished_at=CURRENT_TIMESTAMP(6),active_group=NULL WHERE status='pending' AND `trigger`='alert'
