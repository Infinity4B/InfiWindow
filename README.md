<img src="assets/icon.svg" width="64" height="64" alt="">

# InfiWindow

用 iPad 通过 **UU 远程** 连接 Mac 时，InfiWindow 会自动创建一块和 iPad 分辨率一致的虚拟显示器，远程画面因此能铺满 iPad，不会出现黑边或被拉伸。会话结束后虚拟屏自动移除，显示器排列恢复原样。

默认适配 **iPad Pro 13" (M4)**：2752×2064 px，以 Retina 2x 渲染为 1376×1032 pt。

用 Go 编写，界面基于 [MyGo](https://github.com/egoist/mygo) 原生 UI（GPU 绘制，没有 WebView），不使用 cgo。

## 功能

- **自动检测 UU 远程会话**：检测到有人连入就创建虚拟屏，断开后移除。
- **三种显示方式**
  - **镜像**（默认，推荐）：其他屏幕都镜像虚拟屏。不管 UU 远程采集的是哪块屏幕，iPad 上都能铺满。
  - **主显示器**：虚拟屏成为主显示器（菜单栏和程序坞在上面），其他屏幕保持扩展。
  - **扩展**：只在旁边加一块虚拟屏，需要在 UU 远程里手动切换过去。
- **iPad 型号预设**：Pro 13"/11" (M4)、Pro 12.9"、Air 13"/11"、mini (A17 Pro)；可选横屏或竖屏，60/120 Hz。
- **缩放**：放大 / 默认（Retina）/ 缩小 / 原生 1:1。所有缩放档位都会注册给 macOS，在“系统设置 → 显示器”里也能直接切换。
- **菜单栏常驻**：没有程序坞图标。关闭窗口只是隐藏，可以从菜单栏重新打开、手动创建或移除虚拟屏、切换自动模式。
- **登录时启动**：设为登录项后会在后台启动，不弹出窗口。
- **流量图与活动记录**：实时显示 UU 进程的上行和下行速率以及检测阈值，方便校准。
- **命令行控制**：可以从脚本或快捷指令操作正在运行的实例：

  ```sh
  /Applications/InfiWindow.app/Contents/MacOS/InfiWindow --create   # 创建虚拟屏
  /Applications/InfiWindow.app/Contents/MacOS/InfiWindow --remove   # 移除虚拟屏
  /Applications/InfiWindow.app/Contents/MacOS/InfiWindow --quit     # 退出
  ```

## 原理

**虚拟显示器**：使用 CoreGraphics 内置的 `CGVirtualDisplay`（macOS 11 起提供，DeskPad、BetterDisplay 也用它），通过 [purego](https://github.com/ebitengine/purego) 从纯 Go 调用 Objective-C 运行时。显示器排列用 `kCGConfigureForSession` 修改，即使程序异常退出，注销后也会还原；进程退出时虚拟屏会随之消失。

**会话检测**：UU 远程没有公开 API。InfiWindow 用系统自带的 `nettop`（不需要 root）读取名字以 `UURemote` 开头的进程的网络流量，再用 `lsof` 统计这些进程的对端连接数：

- 上行持续高于**开始阈值**（默认 120 KB/s，持续 2 秒）时，判定为有人连入。只看上行，所以用这台 Mac 去控制别的设备不会触发。
- 上行低于**保持阈值**（默认 6 KB/s）且没有多出来的对端连接，持续**断开等待**时间（默认 25 秒）后，判定为会话结束。

不同网络和画质下流量差别很大。可以先连一次，对照窗口里的流量图，在“会话检测参数”里调整阈值。被监视的进程名前缀也能改，理论上同样适用于其他远程桌面软件。

## 构建

需要 Go 1.27+（MyGo 的命令行工具已作为 Go tool 写进 `go.mod`）。

```sh
go tool mygo dev     # 开发模式运行，修改代码后自动重启
go tool mygo build   # 打包到 build/darwin-arm64/InfiWindow.app 和 .dmg
```

测试：

```sh
CGO_ENABLED=0 go test ./...
# 下面这条会在本机真实创建并移除一块虚拟屏：
CGO_ENABLED=0 go test -tags manual -run ManualCreate -v ./internal/vdisplay
```

## 目录结构

```
main.go, view.go, tray.go     MyGo 应用：窗口、界面、菜单栏
internal/device               iPad 型号预设与分辨率、缩放计算
internal/vdisplay             CGVirtualDisplay 与显示器排列（purego）
internal/detector             nettop/lsof 流量采集与会话状态机
internal/controller           检测结果 → 创建/移除虚拟屏的调度
internal/config               设置读写（~/Library/Application Support/InfiWindow/config.json）
```

## 已知限制

- 只支持 macOS 12 及以上（MyGo 的最低要求）。
- `CGVirtualDisplay` 是私有 API，以后的 macOS 版本可能改动它。
- 会话检测依赖流量判断。画面长时间完全静止且 UU 不发保活数据时，可能被误判为断开，可以调大“断开等待”时间。
- 应用默认使用 ad-hoc 签名。从其他电脑拷贝过来时，首次打开可能需要在“系统设置 → 隐私与安全性”中允许。
