# deploy/grafana/provisioning/dashboards：看板从哪里加载

[shop.yml](shop.yml) 告诉 Grafana：去容器里的 `/var/lib/grafana/dashboards` 目录加载看板，放进名为 OpsDoctor 的文件夹。Compose 把仓库里的 `deploy/grafana/dashboards` 挂载到了这个目录。

看板文件本身在 [deploy/grafana/dashboards](../../dashboards/README.md)。
