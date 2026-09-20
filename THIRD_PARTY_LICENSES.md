# 第三方组件与许可证

本项目自身以 [MIT](LICENSE) 许可证开源。运行/编译时所依赖的第三方组件均为**宽松型许可证**
（MIT / BSD-3-Clause / Apache-2.0），允许商用、修改与再分发，但要求保留版权声明与许可证文本。

## 运行时依赖（编译进 gost-webui 二进制）

| 组件 | 用途 | 许可证 | 版权 |
|---|---|---|---|
| [go.etcd.io/bbolt](https://github.com/etcd-io/bbolt) | 本地数据存储 | MIT | Copyright (c) 2013 Ben Johnson |
| [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml) | 配置文件解析 | MIT / Apache-2.0（含 libyaml 部分） | Copyright (c) 2006-2011 Kirill Simonov；Go 移植部分见仓库 |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | 密码哈希（bcrypt） | BSD-3-Clause | Copyright 2009 The Go Authors |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | 系统调用 | BSD-3-Clause | Copyright 2009 The Go Authors |
| [github.com/skip2/go-qrcode](https://github.com/skip2/go-qrcode) | 客户端链接二维码 | MIT | Copyright (c) 2014 Tom Harwood |

## 外部程序（**未随本项目分发**）

| 组件 | 说明 | 许可证 |
|---|---|---|
| [gost](https://github.com/go-gost/gost) | 实际执行转发/观测/配额的核心程序 | MIT，Copyright (c) 2016 ginuerzh |

> 安装脚本默认从 gost 官方仓库/GitHub Release 下载或在目标机器上自行编译，**本项目仓库与发布包中
> 不包含 gost 的任何源码或二进制**，因此不构成对 gost 的再分发。
>
> 如果你要自行打包分发（例如把 gost 二进制一并放进自己的安装包），请同时附带 gost 的 MIT 许可证文本：
>
> ```
> MIT License
> Copyright (c) 2016 ginuerzh
> ```
>
> 完整文本见 https://github.com/go-gost/gost/blob/master/LICENSE

## 本项目与 gost 的关系

本项目（gost-webui）是**社区第三方工具**，与 gost 官方（ginuerzh / go-gost 组织）**没有隶属或背书关系**；
名称中的 "gost" 仅用于说明它管理的底层程序。如果你修改后二次分发，建议保留此说明以避免混淆。
