# LanRoom Android

Flutter 原生客户端，与仓库内 Go 服务端和网页互通。支持前台自动发现、口令登录、设备列表、群聊/私信、文件上传续传和下载保存。

## 普通用户：安装与使用

电脑先按[根目录首页](../README.md)运行服务，手机安装构建出的 APK。仓库没有承诺提供发布版 APK 下载链接。

```sh
# 从仓库根目录安装本地 debug APK，手机需启用 USB 调试并授权
adb devices
adb install -r mobile/build/app/outputs/flutter-apk/app-debug.apk
```

也可将 APK 复制到手机，按系统提示允许对应来源安装。debug 包仅供开发测试，不代表正式签名发布包；升级出现签名冲突时应先确认签名来源，卸载会丢失本地身份和任务。

打开 App 等待自动发现，按提示选择服务、输入口令，然后与电脑网页互发文字及小文件。具体操作见[使用手册](../docs/usage.md)，发现失败见[排错指南](../docs/troubleshooting.md)。

## 开发者：运行与构建

需要 Flutter 3.47 / Dart 3.13 或兼容版本、JDK 17+、Android SDK 36。最低 Android 7（API 24）。

以下先从仓库根目录开始。可用 `flutter doctor -v` 检查环境、`flutter devices` 查看已授权 Android 设备。先在电脑启动新版服务端：

```sh
go run .
```

在手机与电脑连接同一局域网后：

```sh
cd mobile
flutter pub get
flutter run -d <Android设备ID>
```

构建 APK：

```sh
flutter build apk --debug
```

产物：`build/app/outputs/flutter-apk/app-debug.apk`。这是开发测试包，发布前需要配置正式签名。

共享或只读 Flutter SDK 也可使用以上命令。项目会将 SDK 中的 Gradle 插件源码缓存到 `android/.gradle/flutter-tools/`，在那里编译插件，避免清理 `/opt/dev/flutter` 等 SDK 目录下其他用户的构建产物。缓存按源码内容区分，SDK 更新后自动生成新缓存；删除 `android/.gradle/` 后也会自动重建。不需要使用 `sudo flutter run` 或修改整个 SDK 的权限。

当前启动时会重新发起 UDP 发现，纯手动 HTTPS 地址场景下重启 App 后可能需重新输入地址。首次打开自动发现；一个服务器自动连接，多个服务器手动选择。已选择的服务器按 UUID 记忆，IP 改变后重新发现。找不到原服务器时不会自动连接其他服务器，可通过右上角切换服务器。口令仅在需要时输入，Cookie 和设备密钥使用 Android Keystore 加密存储；服务端重启导致会话失效后需要重新输入口令。

## 网络与 Mihomo

- 默认 UDP 发现端口 `8788`，HTTP 默认 `8787`。防火墙需允许来自局域网的请求；不是 Mihomo 的 `allow-lan` 代理入站设置。
- 广播不经过公网；访客 Wi-Fi、AP 隔离、不同 VLAN 可能阻止发现。
- Go 根据 UDP 入站接口选择回复源地址；手机枚举真实接口与子网广播地址，不使用 TUN 地址广播。
- 仍需操作系统提供到对端的可用路由。Mihomo 接管局域网路由时，应自行检查 `tun.route-exclude-address`；应用不会修改路由、防火墙或代理配置。
- 非默认发现端口的服务器可手动连接；首版 App 自动发现固定使用 UDP `8788`。
- 发现数据没有密码学认证；UUID 仅用于区分服务，不代替 TLS 和房间口令。不应在不可信网络上依赖明文 HTTP 保密。

## 文件和生命周期

系统文件选择器将上传文件流式复制到应用私有目录。上传会话和目标设备持久化，网络中断/重启后点击“继续”；服务端会话过期或重启后会创建新会话重传。取消或完成后清理应用副本。一次只上传一个文件。

下载流式写入临时文件，再使用系统“另存为”导出，不申请全盘存储权限。失败时点击消息重新下载。首版不实现下载断点续传。

连接成功后启动前台服务（`connectedDevice` 类型，通知栏常驻“LanRoom 已连接”），切到后台、锁屏、打开文件选择器时进程不会被冻结，WebSocket 保持在线；传输进行中额外持有 partial wake lock，锁屏也能继续。Android 13+ 首次连接会请求通知权限，拒绝不影响保活，只是看不到通知。从最近任务划掉应用会停止服务并断开。

断线后先直接重连上次的服务器地址（退避 2s→30s），只有直连失败且应用在前台时才回退到 UDP 搜索；回到前台时若 WebSocket 仍在线，不发任何请求。

## 测试

```sh
go test ./...
cd mobile
flutter analyze
flutter test
flutter test integration_test/lan_test.dart -d <Android设备ID>
```

真机集成测试要求：独立测试服务使用 HTTP `18787`、UDP `8788`、口令 `mobile-test-2026`。不要把测试服务口令用于日常服务。电脑运行：

```sh
LANROOM_DATA_DIR=/tmp/lanroom-test-data \
LANROOM_UPLOAD_DIR=/tmp/lanroom-test-uploads \
LANROOM_PIN=mobile-test-2026 go run . -port 18787
```

测试使用手机真实接口发现服务器，不依赖 `adb reverse`。集成测试中的额外客户端是协议测试对端，网页实际互通需另做浏览器验证。测试结果见 [TEST_RESULTS.md](TEST_RESULTS.md)。这些是历史验证记录，不代表当前电脑、手机或网络自动通过；真实地址变更、所有路由器及后台持续在线未获得普遍验证。
