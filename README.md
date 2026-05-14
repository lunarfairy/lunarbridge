# LunarBridge

LunarBridge is a small LAN file transfer app for Windows and macOS. Run it on both computers, open the local browser UI, pair the devices once, then send files or folders directly across the same network.

## Run

```powershell
go run ./cmd/lunarbridge
```

The app starts:

- Local browser UI: `http://127.0.0.1:41230/`
- Peer HTTPS service: `:41231`
- UDP discovery: `:41232`

On first launch, LunarBridge creates its config and receive folder automatically.

## Pair Devices

1. Start LunarBridge on both computers.
2. Put both computers on the same Wi-Fi or LAN.
3. In the device list, choose the other computer.
4. Enter the 6 digit pairing code shown on the other computer.
5. After pairing, choose files or a folder and click Send.

If discovery does not find the other computer, use the manual address shown in the other browser UI, for example:

```text
192.168.1.8:41231
```

Windows/macOS may ask for firewall permission the first time. Allow LunarBridge on private/local networks.

## Troubleshooting

If the UI says it cannot connect to the local LunarBridge service, the program on that computer is not running anymore. Start it again, then refresh `http://127.0.0.1:41230/`.

If pairing says the code is incorrect, enter the 6 digit code shown on the other computer, not the code shown on the computer where you are typing.

If pairing works but sending fails, confirm both programs are still running and allow LunarBridge through the firewall on private/local networks. If automatic discovery looks unstable, manually add the other computer's peer address shown at the top of its page, such as `192.168.1.8:41231`.

## Receive Folder

The default receive folder is:

- Windows: `%USERPROFILE%\Downloads\LunarBridge`
- macOS: `~/Downloads/LunarBridge`

You can change it in the browser UI settings panel.

## Build

Windows PowerShell:

```powershell
.\scripts\build.ps1
```

macOS/Linux shell:

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

## Local Two-Instance Test

Use different config directories and ports:

```powershell
go run ./cmd/lunarbridge -config-dir .tmp\a -ui-port 41230 -peer-port 41231 -discovery-port 41232 -no-browser
go run ./cmd/lunarbridge -config-dir .tmp\b -ui-port 41330 -peer-port 41331 -discovery-port 41332 -no-browser
```

For manual pairing between these two local instances, add:

```text
127.0.0.1:41331
```

from the first UI, or:

```text
127.0.0.1:41231
```

from the second UI.

## Scope

First version supports LAN-only manual transfer, one-time pairing, files and folders, streaming uploads, TLS encryption, and certificate fingerprint pinning. It does not include internet relay, automatic sync, clipboard sync, startup installer, or resumable transfers.
