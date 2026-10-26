# 医生数据库的首次初始化

doctor-mysql 是独立实例。官方镜像先创建 doctor 库和 doctor 账号，再运行这里的脚本，建立 `opsdoctor_doctor_test` 专用测试库。测试不能指向保存真实诊断任务的 doctor 库。

业务表不在这个脚本里建。医生启动时会执行 `internal/doctor/store/migrations` 的迁移，测试也使用同一套迁移。已有数据卷不会自动重跑初始化脚本，需要补测试库时可以由操作者显式执行本脚本。
