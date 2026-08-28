# JourneyIn Mobile Client

移动端使用 Capacitor 包装同一套 web/dist 资源，作为 JourneyIn 服务端的客户端运行。

## 初始化原生工程

在安装 @capacitor/cli 和 @capacitor/core 后，从 mobile 目录执行：

~~~text
npx cap add android
npx cap add ios
npx cap sync
npx cap open android
npx cap open ios
~~~

移动端首次启动需要配置 JourneyIn server URL。移动操作系统的后台生命周期限制意味着移动端不是长期公网服务端；服务端角色由桌面、Linux、服务器或 Docker 实例承担。

地图导航通过系统链接能力打开百度地图/高德地图，未安装 App 时回退 HTTPS。
