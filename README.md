<div align="center">

<img src="./doc/images/logo.png" width="100" height="100" alt="Leon Panel" />

# Leon Panel

基于公开 MIT 源码 Sun-Panel v1.3.0 的独立导航面板。

An independent navigation panel based on the public MIT source of Sun-Panel v1.3.0.

</div>

Leon Panel 是 [Sun-Panel v1.3.0](https://github.com/hslr-s/sun-panel) 公开源码上的独立版本，不是官方仓库，也不是官方 v1.8.1。

- 显示版本：`1.3.0+leon.11`
- 内部数字版本：`10`（原来用来比较版本的数字，这一版保持不变）

版权和 MIT 许可证保留在 [LICENSE](./LICENSE)。上游仓库是 [hslr-s/sun-panel](https://github.com/hslr-s/sun-panel)。本仓库没有改许可证，也没有加入或绕过任何授权检查。实现和差异的详细说明见 [doc/LEON-PANEL.md](./doc/LEON-PANEL.md)。

## 界面预览

下面的图是本仓库自带的 Leon Panel 界面，基于 Sun-Panel v1.3.0，不是官方后续版本的界面。

![Leon Panel 首页，基于 Sun-Panel v1.3.0](./doc/images/main-dark.png)

**图标样式**

![小图标](./doc/images/icon-small-new.png)
![透明详情](./doc/images/transparent-info.png)
![透明小图标](./doc/images/transparent-small.png)
![纯色详情](./doc/images/solid-color-info.png)
![全彩小图标](./doc/images/full-color-small.jpg)

**页面内小窗口**

![SSH 小窗口](./doc/images/window-ssh.png)
![下载工具小窗口](./doc/images/window-xunlei.png)

## 已实现的独立能力

对照 [doc/LEON-PANEL.md](./doc/LEON-PANEL.md)，这一版在 v1.3.0 之上已经有这些能力：

- 界面改进：深色模式下卡片和搜索框有浅色描边，设置页里偏灰的字更亮，手机上的分组按钮不再只靠悬停才出现。已经保存的壁纸、颜色和布局不会被清空。
- 自定义搜索引擎与排序：登录后可以增加、修改、删除自己的搜索引擎，并用上移、下移调整顺序。
- 图片分类、按文件名搜索和分页。升级前已有的文件仍是未分类。
- 批量上传：上传文件管理可以一次选择多张图片。
- 自有图库与公共图库：项目图标和壁纸可以从当前账号自己的图片里选用；管理员可以把自己的图片标成公共，其他已登录账号可以浏览并选用。
- 管理员 SQLite 备份导出：可以下载本程序自己的备份包。
- 整站覆盖恢复：管理员可以从本程序自己的 SQLite 备份包覆盖恢复。恢复会换掉现有数据，页面先说明风险，管理员勾选确认后才会执行。不接受官方备份。MySQL 不能在这里恢复。
- 多账号切换：在这台浏览器记住多个已登录账号，并在系统应用和登录页切换。不保存密码。
- 批量删除：上传文件管理可以多选并删除当前账号自己的图片。只能选择当前页，删除前要再次确认。公共图库不能删除别人的图片。
- 分组卡片样式：每个分组可以单独使用详情大卡片或小图标，也可以跟随风格设置里的全局样式。没有单独设置过的分组仍跟随全局。

## 尚未实现

- OpenAPI
- Docker 套接字控制

## 构建

```sh
pnpm install
pnpm run test:unit
pnpm run type-check
pnpm run build
cd service
go test ./assets ./lib/backup ./lib/pollguard ./initialize/systemMonitor ./api/api_v1/system
```

前端构建结果在 `dist`。服务端需要先按原项目的方式生成嵌入资源，再编译 `service`。

## 独立测试容器

在本仓库目录执行。它使用新的 `./leon-test-data` 和主机端口 `13002`，不挂载正式数据，也不挂载 Docker 套接字。

```sh
docker compose -f docker-compose.leon-test.yml up --build
```

浏览器打开 `http://127.0.0.1:13002`。

## 许可证与上游署名

本仓库使用 MIT 许可证，全文见 [LICENSE](./LICENSE)。

上游项目为 [Sun-Panel](https://github.com/hslr-s/sun-panel)，Copyright (c) 2023 红烧猎人。Leon Panel 基于其公开的 v1.3.0 源码修改，不是官方后续版本。
