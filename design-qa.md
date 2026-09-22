# 幽谷灵境界面检查

final result: passed

## 视觉依据与范围

- 原选定设计：`C:/Users/Administrator/.codex/generated_images/01a0c8c1-e1e7-7022-89ce-ee6291b53236/exec-2085ef7d-163d-428b-b4ae-acf4ba69e5c8.png`，1487 × 1058。
- 用户随后明确要求更亮的蓝天云海、悬山仙殿背景。最新背景：`web/assets/celestial-clouds.webp`，1536 × 1024；背景与标题颜色的变化属于用户要求的调整。
- 本地实现：`http://127.0.0.1:4173/`，独立内存演示数据；实际生产页面仍使用原有 API。
- 最终桌面证据：`.gotmp/nodes-celestial-final.png`，1488 × 1058，CSS viewport 1488 × 1058、DPR 1。
- 最终手机证据：`.gotmp/nodes-celestial-mobile.png`，CSS viewport 390 × 844、DPR 1；滚动条占 15px，可用宽度 375px，document scrollWidth 375px，无页面横向溢出。
- 全景并排比较：`.gotmp/design-comparison-celestial.jpg`。参考与实现按比例缩小放在同一画布中，未改变长宽比；1px 的源宽度差忽略。
- 节点行局部比较：`.gotmp/design-detail-comparison.jpg`；背景替换前对同一布局的字体、间距、状态、配额与操作区域进行对照。最终桌面完整截图进一步确认同一组件未受背景替换影响。

## 检查结论

- 字体：标题采用本地宋体类衬线回退，正文沿用项目系统无衬线栈，数字采用现有等宽字体。未新增外部字体请求。桌面与手机的标题、名称和数据可读；手机登录文案改为明确两行，避免单字换行。
- 布局：实现顶部横向导航、景观标题区、主节点列表和右侧详情面板。手机使用紧凑列表，监听端口与配额保留在选中节点详情中，开关与操作按钮直接可见。
- 色彩：墨绿数据表面、翡翠主按钮和开关、淡金配额条；新背景几乎没有遮罩，景观上文字改为深青色。浅色模式保留浅玉色数据表面。
- 图片：背景为实际生成并压缩的本地 WebP，不是占位图；白昼蓝天、浮山、仙殿与瀑布清楚可见。未把整张界面截图当作网页。复用已有应用图标。
- 内容：数值全部来自原有 API 字段。设计稿里的实时带宽改为已有的今日上下行流量；保留落地机信息、协议、配额与原有操作菜单。样例数值与设计稿不同是预览数据差异，不是假定生产数据。

## 修复与复查历史

1. 初次实现发现顶部导航继承纵向排列、详情卡继承 24px 相邻外边距（P1/P2）。显式设置横向排列，清除网格详情卡外边距，收紧表格行高与详情间距。复查证据：`.gotmp/design-comparison-final.jpg`。
2. 手机端原表格操作需要横向滚动（P2）。改为名称、状态、操作三列，详细数据下移到详情。复查证据：`.gotmp/nodes-mobile-final.png`、最新手机截图。
3. 主表面 backdrop-filter 导致 fixed 菜单定位偏移（P1）。关闭该表面的 backdrop-filter，保留透明背景色。复测菜单复制链接成功，出现“已复制到剪贴板”。
4. 弹窗焦点未进入弹窗（P2）。添加焦点进入与返回、背景 inert、Tab 循环和 Escape 关闭；可访问性树确认焦点位于“关闭弹窗”。
5. 登录页手机文案尾字单独换行、图表最右侧日期被截（P2）。恢复两行文案，并增加图表右侧绘图留白；登录复查证据：`.gotmp/login-mobile-final.png`。
6. 用户要求更明亮背景。生成并接入云海仙殿，删除深色遮罩，调整景观区文字；桌面和手机重新截图验证，最新证据见上。

## 功能与构建验证

- 浏览器验证：四个页面导航、节点选择和详情同步、节点启停、菜单复制链接、添加/编辑弹窗、GOST/链接模式切换、Escape、浅深主题切换、登录/退出界面（独立演示环境）。
- 额外截图：`.gotmp/node-modal.png`、`.gotmp/nodes-light.png`、`.gotmp/notify-mobile.png`、`.gotmp/system-mobile.png`、`.gotmp/login-desktop.png`、`.gotmp/overview-desktop.png`。
- 浏览器 error/warn 日志为空。
- `node --check web/app.js`、`git diff --check` 通过。
- Linux amd64 目标 `go build` 通过，包含新静态资源。
- Windows 下完整 `go test ./...` 被既有 Linux 专用 syscall.Statfs/Setpgid 接口阻止；link/store 测试通过。未在真实 Linux/GOST 环境执行端到端节点创建、转发、证书或通知测试。

## 可选细化

- 保留现有图标与操作菜单，不复刻设计稿中虚构的标志和装饰角花；不影响界面使用与主题一致性。
- 背景文件与完整生成简述见 `web/assets/README.md`。

所有本轮发现的 P0/P1/P2 界面问题已修复。正式服务行为仍需在 Linux 部署环境验证。

## 1.5.0 发布前复查

- 按用户反馈完全移除全页遮罩；浏览器确认浅色主题下 body::after 的 content 为 none，背景图片保持原色，组件主题切换正常。
- 版本更新为 1.5.0；将 main.version 改为变量，使 release.sh / Makefile 的 -X 链接器版本参数实际生效；Makefile 默认读取源码版本。
- JavaScript 语法、diff 空白检查、link/store 测试及 Linux 目标 go vet 通过。
