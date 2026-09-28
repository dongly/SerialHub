# 升级源采用 GitHub Releases 而非自建 git 服务器

SerialHub 有两个远端：GitHub（github.com/dongly/SerialHub）与自建 git 服务器（ssh://git@tj28.top:1122）。`serialhub upgrade` 的版本查询与二进制下载选择 GitHub Releases 作为唯一升级源：GitHub Releases 提供稳定的 REST API（最新 tag、资产列表、下载 URL）、资产可附 sha256 校验文件、且 `.github/workflows/release.yml` 已实现 push tag 自动构建发布（Linux/Windows amd64 + release notes）。当前项目在自建服务器上未配置/未使用相应发布能力，做自动升级需要另建一套服务。

## Consequences

- 需要访问 GitHub 网络（可用 `HTTPS_PROXY` 代理；`SERIALHUB_GITHUB_API` 仅覆盖 API 基址，资产下载仍按响应中的 `browser_download_url` 进行）。
- 升级强制 sha256 校验：Release 缺失 `.sha256` 资产时拒绝升级（安全优先，不降级安装）。
