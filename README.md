# LunarBridge

## 简介 | Overview

**中文**

LunarBridge 是一个面向 Windows 和 macOS 的局域网文件互传工具。两台电脑都运行程序后，打开本机浏览器界面，完成一次配对，就可以在同一网络里直接发送文件或文件夹。

**English**

LunarBridge is a LAN file transfer app for Windows and macOS. Run it on both computers, open the local browser UI, pair the devices once, then send files or folders directly across the same network.

## 快速开始 | Quick Start

**中文**

运行：

```powershell
go run ./cmd/lunarbridge
```

程序会启动：

- 本地浏览器界面：`http://127.0.0.1:41230/`
- 设备互传 HTTPS 服务：`:41231`
- UDP 自动发现：`:41232`

第一次启动时，LunarBridge 会自动创建配置目录和接收目录。

**English**

Run:

```powershell
go run ./cmd/lunarbridge
```

The app starts:

- Local browser UI: `http://127.0.0.1:41230/`
- Peer HTTPS service: `:41231`
- UDP discovery: `:41232`

On first launch, LunarBridge creates its config and receive folder automatically.

## 配对与发送 | Pair and Send

**中文**

1. 在两台电脑上启动 LunarBridge。
2. 让两台电脑连接到同一个 Wi-Fi 或局域网。
3. 在设备列表中选择另一台电脑。
4. 输入对方页面上显示的 6 位配对码。
5. 配对成功后，选择文件或文件夹，点击发送。

如果自动发现没有找到对方，可以手动输入对方页面显示的地址，例如：

```text
192.168.1.8:41231
```

Windows 或 macOS 第一次可能会弹出防火墙提示，允许 LunarBridge 在专用网络或本地网络中通信即可。

**English**

1. Start LunarBridge on both computers.
2. Put both computers on the same Wi-Fi or LAN.
3. Choose the other computer in the device list.
4. Enter the 6 digit pairing code shown on the other computer.
5. After pairing, choose files or a folder and click Send.

If discovery does not find the other computer, use the manual address shown in the other browser UI, for example:

```text
192.168.1.8:41231
```

Windows or macOS may ask for firewall permission the first time. Allow LunarBridge on private or local networks.

## 故障排查 | Troubleshooting

**中文**

- 如果页面提示无法连接本机 LunarBridge 服务，说明这台电脑上的程序已经退出。重新启动程序，再刷新 `http://127.0.0.1:41230/`。
- 如果提示配对码错误，输入的是“对方电脑页面上显示的 6 位码”，不是当前这台电脑自己的码。
- 如果已经配对成功但发送失败，确认两边程序都还在运行，并且防火墙允许 LunarBridge 通信。
- 如果自动发现不稳定，可以手动添加对方页面顶部显示的互传地址，例如 `192.168.1.8:41231`。

**English**

- If the UI says it cannot connect to the local LunarBridge service, the program on that computer is no longer running. Start it again, then refresh `http://127.0.0.1:41230/`.
- If pairing says the code is incorrect, enter the 6 digit code shown on the other computer, not the code shown on the computer where you are typing.
- If pairing works but sending fails, confirm both programs are still running and that the firewall allows LunarBridge.
- If automatic discovery looks unstable, manually add the other computer's peer address shown at the top of its page, such as `192.168.1.8:41231`.

## 接收目录 | Receive Folder

**中文**

默认接收目录：

- Windows：`%USERPROFILE%\Downloads\LunarBridge`
- macOS：`~/Downloads/LunarBridge`

你可以在浏览器界面的设置面板中修改它。

**English**

The default receive folder is:

- Windows: `%USERPROFILE%\Downloads\LunarBridge`
- macOS: `~/Downloads/LunarBridge`

You can change it in the browser UI settings panel.

## 构建 | Build

**中文**

Windows PowerShell：

```powershell
.\scripts\build.ps1
```

macOS 或 Linux shell：

```sh
sh ./scripts/build.sh
```

构建产物：

- `dist/lunarbridge-windows-amd64.exe`
- `dist/lunarbridge-darwin-amd64`
- `dist/lunarbridge-darwin-arm64`

把 macOS 二进制拷到 Mac 后，先执行：

```sh
chmod +x ./lunarbridge-darwin-arm64
./lunarbridge-darwin-arm64
```

如果是 Intel Mac，请把文件名换成 `lunarbridge-darwin-amd64`。

**English**

Windows PowerShell:

```powershell
.\scripts\build.ps1
```

macOS or Linux shell:

```sh
sh ./scripts/build.sh
```

Outputs:

- `dist/lunarbridge-windows-amd64.exe`
- `dist/lunarbridge-darwin-amd64`
- `dist/lunarbridge-darwin-arm64`

After copying a macOS binary to the Mac, make it executable before running:

```sh
chmod +x ./lunarbridge-darwin-arm64
./lunarbridge-darwin-arm64
```

If you are using an Intel Mac, use `lunarbridge-darwin-amd64` instead.

## 本地双实例测试 | Local Two-Instance Test

**中文**

可以用不同的配置目录和端口在一台机器上启动两个实例：

```powershell
go run ./cmd/lunarbridge -config-dir .tmp\a -ui-port 41230 -peer-port 41231 -discovery-port 41232 -no-browser
go run ./cmd/lunarbridge -config-dir .tmp\b -ui-port 41330 -peer-port 41331 -discovery-port 41332 -no-browser
```

此时可以手动输入：

- 第一个界面填 `127.0.0.1:41331`
- 第二个界面填 `127.0.0.1:41231`

**English**

Use different config directories and ports to start two instances on one machine:

```powershell
go run ./cmd/lunarbridge -config-dir .tmp\a -ui-port 41230 -peer-port 41231 -discovery-port 41232 -no-browser
go run ./cmd/lunarbridge -config-dir .tmp\b -ui-port 41330 -peer-port 41331 -discovery-port 41332 -no-browser
```

For manual pairing between these two local instances:

- Use `127.0.0.1:41331` from the first UI
- Use `127.0.0.1:41231` from the second UI

## 当前范围 | Current Scope

**中文**

第一版支持：

- 仅限局域网手动互传
- 一次配对后复用信任关系
- 文件和文件夹发送
- 流式上传
- TLS 加密
- 证书指纹固定校验

暂不支持：

- 公网中转
- 自动同步
- 剪贴板同步
- 安装器或开机自启
- 断点续传

**English**

The first version supports:

- LAN-only manual transfer
- One-time pairing with remembered trust
- File and folder transfer
- Streaming uploads
- TLS encryption
- Certificate fingerprint pinning

It does not currently include:

- Internet relay
- Automatic sync
- Clipboard sync
- Installer or auto-start integration
- Resumable transfers
