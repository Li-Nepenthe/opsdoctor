// 本文件：在 CI 的专用 MySQL 库对比旧 Apply 与 golang-migrate v4.20.1。
//
// 先运行全量 SQL，再观察接管、竞争、取消和历史保护。所有用例串行共享专用测试库，
// 只有明确标出的两个启动者并发。准备失败会终止测试，库的行为差异则写入 MIGRATE_PROBE，
// 不把“候选方案已兼容”预先写成断言；这些探针通过不代表生产替换已经通过验收。

package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	gmdb "github.com/golang-migrate/migrate/v4/database"
)

// 探针表采用非幂等 INSERT：重跑旧版本会撞主键，不能被 IF NOT EXISTS 掩盖。
const (
	probeCreate    = "CREATE TABLE probe_marks (id INT PRIMARY KEY)"
	probeInsertOne = "INSERT INTO probe_marks VALUES (1)"
	probeInsertTwo = "INSERT INTO probe_marks VALUES (2)"
)

// TestIntegrationMigrateProbeFresh 比较完整生产 SQL 在两种迁移器下的最终表结构。
func TestIntegrationMigrateProbeFresh(t *testing.T) {
	for _, source := range []struct {
		name, directory string
		count           int
	}{
		{"doctor", "../../doctor/store/migrations", 42}, // 第 43 个目录文件是 README，不是 SQL。
		{"shop", "../../shop/migrations", 5},
	} {
		t.Run(source.name, func(t *testing.T) {
			db := probeDatabase(t)
			files := probeRepositoryFiles(t, source.directory, source.count)
			_, oldMulti := db.ExecContext(t.Context(), "SELECT 1; SELECT 2")
			if oldMulti == nil {
				t.Fatal("探针连接意外开启多语句")
			}
			if err := Apply(t.Context(), db, files, "."); err != nil {
				t.Fatal("旧实现全量准备失败：", probeError(err))
			}
			oldShape := probeSchema(t, db)
			old := map[string]any{"up": "nil", "repeat": probeError(Apply(t.Context(), db, files, ".")), "history_rows": probeCount(t, db, "SELECT COUNT(*) FROM schema_migrations"), "schema": oldShape, "multiple_statements": probeError(oldMulti)}
			probeReset(t, db)
			library := probeMustOpen(t, t.Context(), db, files, "schema_version")
			_, newMulti := library.connection.ExecContext(t.Context(), "SELECT 1; SELECT 2")
			if newMulti == nil {
				t.Fatal("WithConnection 后意外开启多语句")
			}
			up := library.migration.Up()
			shape := probeSchema(t, db)
			current := map[string]any{"up": probeError(up), "repeat": probeError(library.migration.Up()), "state": probeState(library), "schema": shape, "multiple_statements": probeError(newMulti)}
			probeLog(t, "fresh/"+source.name, map[string]any{"files": len(files), "old": old, "library": current, "same_schema": oldShape["sha256"] == shape["sha256"]})
		})
	}
}

// TestIntegrationMigrateProbeLegacy 观察默认表冲突，再用不同表名和条件插入接管完整旧库。
func TestIntegrationMigrateProbeLegacy(t *testing.T) {
	for _, source := range []struct {
		name, directory string
		count           int
	}{
		{"doctor", "../../doctor/store/migrations", 42}, {"shop", "../../shop/migrations", 5},
	} {
		t.Run(source.name, func(t *testing.T) {
			db := probeDatabase(t)
			files := probeRepositoryFiles(t, source.directory, source.count)
			if err := Apply(t.Context(), db, files, "."); err != nil {
				t.Fatal(probeError(err))
			}
			oldRows := probeCount(t, db, "SELECT COUNT(*) FROM schema_migrations")
			if oldRows != int64(len(files)) {
				t.Fatal("旧库历史未覆盖全部 SQL")
			}
			shape := probeSchema(t, db)
			oldRepeat := Apply(t.Context(), db, files, ".")
			// 默认表名与旧表相同；分开记录构造器和 Up，避免把失败阶段说错。
			direct, initErr := probeOpen(t, t.Context(), db, files, "")
			directUp := "not_run"
			if direct != nil {
				directUp = probeError(direct.migration.Up())
				direct.close(t)
			}
			adopted := probeMustOpen(t, t.Context(), db, files, "schema_version")
			rows, adoptErr := probeAdopt(t.Context(), adopted.connection, source.count)
			up := "not_run"
			if adoptErr == nil {
				up = probeError(adopted.migration.Up())
			}
			againRows, againErr := probeAdopt(t.Context(), adopted.connection, source.count)
			probeLog(t, "legacy/"+source.name, map[string]any{
				"old":             map[string]any{"repeat": probeError(oldRepeat), "history_rows": oldRows},
				"library_default": map[string]any{"initialize": probeError(initErr), "up": directUp},
				"candidate":       map[string]any{"adopt_error": probeError(adoptErr), "adopt_rows": rows, "up": up, "repeat_adopt_error": probeError(againErr), "repeat_adopt_rows": againRows, "state": probeState(adopted), "old_history_rows": probeCount(t, db, "SELECT COUNT(*) FROM schema_migrations"), "same_schema": shape["sha256"] == probeSchema(t, db)["sha256"], "old_repeat_after_adoption": probeError(Apply(t.Context(), db, files, "."))},
			})
		})
	}
}

// TestIntegrationMigrateProbeConcurrentTakeover 对比两个启动者，并重复候选接管竞争以观察偶发冲突。
func TestIntegrationMigrateProbeConcurrentTakeover(t *testing.T) {
	t.Run("old", func(t *testing.T) {
		db := probeDatabase(t)
		if err := Apply(t.Context(), db, probeFiles(probeCreate, probeInsertOne), "."); err != nil {
			t.Fatal(probeError(err))
		}
		// 起跑屏障使两个调用面对同一份旧历史；每个工作者只写自己的结果槽位。
		start := make(chan struct{})
		results := make([]string, 2)
		var workers sync.WaitGroup
		for index := range 2 {
			workers.Go(func() {
				<-start
				results[index] = probeError(Apply(t.Context(), db, probeFiles(probeCreate, probeInsertOne, probeInsertTwo), "."))
			})
		}
		close(start)
		workers.Wait()
		probeLog(t, "concurrent/old", map[string]any{"results": results, "marks": probeCount(t, db, "SELECT COUNT(*) FROM probe_marks"), "history_rows": probeCount(t, db, "SELECT COUNT(*) FROM schema_migrations")})
	})
	for round := 1; round <= 10; round++ {
		t.Run(fmt.Sprintf("candidate_%02d", round), func(t *testing.T) {
			db := probeDatabase(t)
			base := probeFiles(probeCreate, probeInsertOne)
			if err := Apply(t.Context(), db, base, "."); err != nil {
				t.Fatal(probeError(err))
			}
			files := probeFiles(probeCreate, probeInsertOne, probeInsertTwo)
			instances := []*probeLibrary{probeMustOpen(t, t.Context(), db, files, "schema_version"), probeMustOpen(t, t.Context(), db, files, "schema_version")}
			start := make(chan struct{})
			results := make([]map[string]any, 2)
			var workers sync.WaitGroup
			for index, instance := range instances {
				workers.Go(func() {
					<-start
					rows, err := probeAdopt(t.Context(), instance.connection, 2)
					up := "not_run_after_adopt_error"
					// 只按方案把主键冲突视作对方已接管；死锁等其他错误原样记录，不偷偷加重试。
					if err == nil || probeError(err) == "mysql.1062" {
						up = probeError(instance.migration.Up())
					}
					results[index] = map[string]any{"adopt_error": probeError(err), "adopt_rows": rows, "up": up}
				})
			}
			close(start)
			workers.Wait()
			probeLog(t, fmt.Sprintf("concurrent/candidate_%02d", round), map[string]any{"results": results, "state": probeState(instances[0]), "version_rows": probeCount(t, db, "SELECT COUNT(*) FROM schema_version"), "marks": probeCount(t, db, "SELECT COUNT(*) FROM probe_marks"), "old_history_rows": probeCount(t, db, "SELECT COUNT(*) FROM schema_migrations")})
		})
	}
	// 另做确定性的迟到者：A 已推进到 3，B 仍拿着旧版本 2，再接管不能让版本倒退。
	t.Run("stale_version", func(t *testing.T) {
		db := probeDatabase(t)
		if err := Apply(t.Context(), db, probeFiles(probeCreate, probeInsertOne), "."); err != nil {
			t.Fatal(probeError(err))
		}
		files := probeFiles(probeCreate, probeInsertOne, probeInsertTwo)
		first := probeMustOpen(t, t.Context(), db, files, "schema_version")
		late := probeMustOpen(t, t.Context(), db, files, "schema_version")
		_, err := probeAdopt(t.Context(), first.connection, 2)
		if err != nil {
			t.Fatal(probeError(err))
		}
		firstUp := first.migration.Up()
		rows, lateErr := probeAdopt(t.Context(), late.connection, 2)
		lateUp := late.migration.Up()
		probeLog(t, "concurrent/stale_version", map[string]any{"first_up": probeError(firstUp), "late_adopt": probeError(lateErr), "late_rows": rows, "late_up": probeError(lateUp), "state": probeState(late), "marks": probeCount(t, db, "SELECT COUNT(*) FROM probe_marks")})
	})
}

// TestIntegrationMigrateProbeLockWait 实际持锁 12 秒，分别观察旧实现、Up 和构造器的等待与重试。
func TestIntegrationMigrateProbeLockWait(t *testing.T) {
	for _, mode := range []string{"old", "library_up", "library_initialize"} {
		t.Run(mode, func(t *testing.T) {
			db := probeDatabase(t)
			files := probeFiles(probeCreate)
			var library *probeLibrary
			if mode == "library_up" {
				library = probeMustOpen(t, t.Context(), db, files, "schema_version")
			}
			holder, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(probeError(err))
			}
			defer func() {
				if err := closeConnection(holder); err != nil {
					t.Error(probeError(err))
				}
			}()
			var name string
			if mode == "old" {
				name, err = acquire(t.Context(), holder)
			} else {
				// 使用库公开的同一锁名生成函数，确保争用的是该版本真正使用的命名锁。
				name, err = gmdb.GenerateAdvisoryLockId("opsdoctor_migrate_test:schema_version")
				if err == nil {
					var acquired int
					err = holder.QueryRowContext(t.Context(), "SELECT GET_LOCK(?, 0)", name).Scan(&acquired)
					if err == nil && acquired != 1 {
						err = fmt.Errorf("探针未持有锁")
					}
				}
			}
			if err != nil {
				t.Fatal(probeError(err))
			}
			released := make(chan error, 1)
			go func() {
				select {
				case <-time.After(12 * time.Second):
				case <-t.Context().Done():
				}
				released <- release(holder, name)
			}()
			started := time.Now()
			var first, retry error
			retried := false
			if mode == "old" {
				first = Apply(t.Context(), db, files, ".")
			} else if mode == "library_up" {
				first = library.migration.Up()
			} else {
				library, first = probeOpen(t, t.Context(), db, files, "schema_version")
			}
			firstMillis := time.Since(started).Milliseconds()
			if errors.Is(first, gmdb.ErrLocked) {
				retried = true
				if mode == "library_up" {
					retry = library.migration.Up()
				} else {
					library, retry = probeOpen(t, t.Context(), db, files, "schema_version")
				}
			}
			totalMillis := time.Since(started).Milliseconds()
			// 先等释放工作者退出再结束用例；主线程与清理不会同时操作 holder。
			if err := <-released; err != nil {
				t.Fatal("持锁连接释放失败：", probeError(err))
			}
			observation := map[string]any{"hold_ms": 12000, "first": probeError(first), "first_ms": firstMillis, "retried": retried, "retry": probeError(retry), "total_ms": totalMillis}
			if library != nil {
				observation["state"] = probeState(library)
			}
			probeLog(t, "lock_wait/"+mode, observation)
		})
	}
}

// TestIntegrationMigrateProbeDirty 观察失败、再次启动和修正 SQL 后的区别，不调用 Force。
func TestIntegrationMigrateProbeDirty(t *testing.T) {
	for _, mode := range []string{"old", "library"} {
		t.Run(mode, func(t *testing.T) {
			db := probeDatabase(t)
			bad := probeFiles(probeCreate, "INSER INTO probe_marks VALUES (1)", probeInsertTwo)
			fixed := probeFiles(probeCreate, probeInsertOne, probeInsertTwo)
			var first, second, repaired error
			var state map[string]any
			if mode == "old" {
				first = Apply(t.Context(), db, bad, ".")
				second = Apply(t.Context(), db, bad, ".")
				state = map[string]any{"history_rows": probeCount(t, db, "SELECT COUNT(*) FROM schema_migrations")}
				repaired = Apply(t.Context(), db, fixed, ".")
			} else {
				library := probeMustOpen(t, t.Context(), db, bad, "schema_version")
				first = library.migration.Up()
				second = library.migration.Up()
				state = probeState(library)
				library.close(t)
				// 新建实例排除内存状态影响；只修改内存 SQL，没有改生产迁移文件。
				corrected := probeMustOpen(t, t.Context(), db, fixed, "schema_version")
				repaired = corrected.migration.Up()
			}
			probeLog(t, "dirty/"+mode, map[string]any{"first": probeError(first), "second": probeError(second), "after_failure": state, "after_sql_fix": probeError(repaired), "marks": probeCount(t, db, "SELECT COUNT(*) FROM probe_marks")})
		})
	}
}

// probeSleeping 等到真实 SQL 已进入 SLEEP，再取消 context，避免只测到启动前取消。
func probeSleeping(t *testing.T, db *sql.DB) bool {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		// 同账号能看到自己的连接；排除观察查询自身，不需要额外 PROCESS 权限。
		var count int
		err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID <> CONNECTION_ID() AND DB = DATABASE() AND INFO LIKE 'SELECT SLEEP(2)%migrate_probe_cancel%'").Scan(&count)
		if err != nil {
			return false
		}
		if count > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// TestIntegrationMigrateProbeCancellation 在正在执行 SQL 时取消，另观测显式 GracefulStop 的候选桥接。
func TestIntegrationMigrateProbeCancellation(t *testing.T) {
	for _, mode := range []string{"old", "library", "library_graceful"} {
		t.Run(mode, func(t *testing.T) {
			db := probeDatabase(t)
			files := probeFiles(probeCreate, "SELECT SLEEP(2) /* migrate_probe_cancel */", probeInsertOne)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var library *probeLibrary
			if mode != "old" {
				library = probeMustOpen(t, ctx, db, files, "schema_version")
			}
			done := make(chan error, 1)
			go func() {
				if mode == "old" {
					done <- Apply(ctx, db, files, ".")
				} else {
					done <- library.migration.Up()
				}
			}()
			observed := probeSleeping(t, db)
			started := time.Now()
			cancel()
			// 普通 library 不添加取消适配，观察 WithConnection 的 ctx 是否传到后续 Up。
			if mode == "library_graceful" {
				library.migration.GracefulStop <- true
			}
			err := <-done
			elapsed := time.Since(started).Milliseconds()
			if !observed {
				t.Fatal("未观察到正在执行的探针 SQL，不能据此判断取消行为")
			}
			state := map[string]any{}
			if library != nil {
				state = probeState(library)
			} else {
				state["history_rows"] = probeCount(t, db, "SELECT COUNT(*) FROM schema_migrations")
			}
			probeLog(t, "cancellation/"+mode, map[string]any{"sql_observed_running": observed, "context": probeError(ctx.Err()), "up": probeError(err), "after_cancel_ms": elapsed, "state": state, "marks": probeCount(t, db, "SELECT COUNT(*) FROM probe_marks")})
		})
	}
}

// TestIntegrationMigrateProbeChangedFile 在已执行后改动同名文件，观察是否仍核对历史内容。
func TestIntegrationMigrateProbeChangedFile(t *testing.T) {
	for _, mode := range []string{"old", "library"} {
		t.Run(mode, func(t *testing.T) {
			db := probeDatabase(t)
			original := probeFiles(probeCreate, probeInsertOne)
			changed := probeFiles(probeCreate, probeInsertTwo)
			var result error
			if mode == "old" {
				if err := Apply(t.Context(), db, original, "."); err != nil {
					t.Fatal(probeError(err))
				}
				result = Apply(t.Context(), db, changed, ".")
			} else {
				library := probeMustOpen(t, t.Context(), db, original, "schema_version")
				if err := library.migration.Up(); err != nil {
					t.Fatal(probeError(err))
				}
				library.close(t)
				// 重启后用改过的文件重新建立 source，避免缓存掩盖文件变化。
				restarted := probeMustOpen(t, t.Context(), db, changed, "schema_version")
				result = restarted.migration.Up()
			}
			probeLog(t, "changed_file/"+mode, map[string]any{"result": probeError(result), "marks": probeCount(t, db, "SELECT COUNT(*) FROM probe_marks"), "changed_row_present": probeCount(t, db, "SELECT COUNT(*) FROM probe_marks WHERE id = 2")})
		})
	}
}
