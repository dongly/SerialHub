package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/pkg/version"
)

// upgrader 参数均可注入（测试用 httptest 与临时目录替换）。
type upgrader struct {
	apiBase string // GitHub API 基址，默认 https://api.github.com（可用 SERIALHUB_GITHUB_API 覆盖）
	repo    string // 仓库名 owner/repo
	goos    string
	goarch  string
	exePath string // 当前二进制路径
	nowVer  string // 当前版本（不含 hash 后缀）
	client  *http.Client
}

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: i18n.CLI.UpgradeShort,
		Long:  i18n.CLI.UpgradeLong,
		RunE:  runUpgrade,
		Args:  cobra.NoArgs,
	}
	return cmd
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	apiBase := os.Getenv("SERIALHUB_GITHUB_API")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf(i18n.CLI.LocateExe, err)
	}
	u := &upgrader{
		apiBase: strings.TrimSuffix(apiBase, "/"),
		repo:    "dongly/SerialHub",
		goos:    runtime.GOOS,
		goarch:  runtime.GOARCH,
		exePath: exe,
		nowVer:  version.Version,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
	return u.run(os.Stdout)
}

// ghAsset 是 release 资产的最小字段。
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ghRelease 是 releases/latest 响应的最小字段。
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

func (u *upgrader) run(out io.Writer) error {
	// 显示用完整版本（含构建哈希，与 --version 一致）；比较逻辑仍用 u.nowVer。
	fmt.Fprintf(out, i18n.CLI.CheckVersion, appVersion)
	rel, err := u.fetchLatestRelease()
	if err != nil {
		return fmt.Errorf(i18n.CLI.FetchFailed, err)
	}
	latestVer := trimVPrefix(rel.TagName)
	if compareVersion(latestVer, u.nowVer) <= 0 {
		fmt.Fprintf(out, i18n.CLI.AlreadyLatest, appVersion, rel.TagName)
		return nil
	}

	asset, sumAsset, err := u.pickAssets(rel, latestVer)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, i18n.CLI.FoundVersion, latestVer, appVersion, asset.Name)

	data, err := u.download(asset.BrowserDownloadURL, maxArchiveBytes)
	if err != nil {
		return fmt.Errorf(i18n.CLI.DownloadFailed, asset.Name, err)
	}
	want, err := u.download(sumAsset.BrowserDownloadURL, maxSumBytes)
	if err != nil {
		return fmt.Errorf(i18n.CLI.DownloadChecksumFailed, err)
	}
	if err := verifySHA256(data, want); err != nil {
		return fmt.Errorf(i18n.CLI.VerifyFailed, err)
	}
	fmt.Fprintln(out, i18n.CLI.ChecksumOK)

	bin, err := extractBinary(data, u.goos)
	if err != nil {
		return fmt.Errorf(i18n.CLI.ExtractFailed, asset.Name, err)
	}
	if err := u.replaceSelf(bin); err != nil {
		return fmt.Errorf(i18n.CLI.ReplaceFailed, err)
	}
	fmt.Fprintf(out, i18n.CLI.UpgradeDone, appVersion, latestVer)
	return nil
}

func (u *upgrader) fetchLatestRelease() (*ghRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", u.apiBase, u.repo)
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf(i18n.UpgradeErrors.APIStatus, resp.Status)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIBytes)).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, errors.New(i18n.UpgradeErrors.MissingTag)
	}
	return &rel, nil
}

// pickAssets 选出当前平台的压缩包资产及其 .sha256 校验文件。
func (u *upgrader) pickAssets(rel *ghRelease, ver string) (asset, sumAsset *ghAsset, err error) {
	ext := ".tar.gz"
	if u.goos == "windows" {
		ext = ".zip"
	}
	name := fmt.Sprintf("serialhub-%s-%s-%s%s", ver, u.goos, u.goarch, ext)
	for i := range rel.Assets {
		a := &rel.Assets[i]
		switch a.Name {
		case name:
			asset = a
		case name + ".sha256":
			sumAsset = a
		}
	}
	if asset == nil {
		return nil, nil, fmt.Errorf(i18n.UpgradeErrors.MissingAsset, rel.TagName, name)
	}
	if sumAsset == nil {
		return nil, nil, fmt.Errorf(i18n.UpgradeErrors.MissingChecksum, rel.TagName, name)
	}
	return asset, sumAsset, nil
}

// 下载大小上限：压缩包 128MB、校验文件 4KB、API 响应 1MB。
// 防止异常代理响应或损坏的 release 资产耗尽内存。
// 压缩包下载限 maxArchiveBytes，解压出的二进制另限 maxExtractedBytes
// （高压缩率归档的解压内容可远大于压缩包本身）。
// maxExtractedBytes 是 var 而非 const，便于测试注入小值验证超限路径。
var maxExtractedBytes int64 = 256 << 20

// maxArchiveStreamBytes 限制 tar 归档的累计解压流量（顺序解压格式下跳过
// 非目标条目也要解压其数据）。var 便于测试注入小值。
var maxArchiveStreamBytes int64 = 512 << 20

// limitReader 限制底层读取的总字节数，超限返回错误（防解压炸弹类流量）。
type limitReader struct {
	r         io.Reader
	remaining int64
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		return 0, fmt.Errorf(i18n.UpgradeErrors.ArchiveLimit, maxArchiveStreamBytes)
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.r.Read(p)
	l.remaining -= int64(n)
	return n, err
}

const (
	maxArchiveBytes = 128 << 20
	maxSumBytes     = 4 << 10
	maxAPIBytes     = 1 << 20
)

// download 下载 url 内容，超过 max 字节报错（限制下载体积；解压内容另有上限）。
// API 响应经 maxAPIBytes 限制解码输入长度（json.Decoder 只取首个 JSON 值，
// 完整 JSON 后附大量尾随数据的场景不在此拦截）。
func (u *upgrader) download(url string, max int64) ([]byte, error) {
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf(i18n.UpgradeErrors.DownloadStatus, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf(i18n.UpgradeErrors.ResponseLimit, max)
	}
	return data, nil
}

// verifySHA256 比对数据与 .sha256 文件内容（首字段十六进制摘要）。
func verifySHA256(data, sumFile []byte) error {
	fields := strings.Fields(string(sumFile))
	if len(fields) == 0 {
		return errors.New(i18n.UpgradeErrors.EmptyChecksum)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, fields[0]) {
		return fmt.Errorf(i18n.UpgradeErrors.DigestMismatch, fields[0], got)
	}
	return nil
}

// extractBinary 从发布压缩包（按 goos 选格式）中解出 serialhub 可执行文件内容。
// 发布包内是 serialhub-<ver>-<os>-<arch>/serialhub(.exe) 目录结构，按文件名匹配。
// 只接受普通文件（拒绝目录与符号链接/硬链接条目，防止打包异常把空文件或链接
// 文本装进可执行路径）；空内容与多个同名候选同样拒绝。
func extractBinary(data []byte, goos string) ([]byte, error) {
	want := "serialhub"
	if goos == "windows" {
		want += ".exe"
	}
	if goos == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		var candidates []*zip.File
		for _, f := range zr.File {
			if filepath.Base(f.Name) == want && f.Mode().IsRegular() {
				candidates = append(candidates, f)
			}
		}
		if len(candidates) == 0 {
			return nil, fmt.Errorf(i18n.UpgradeErrors.NoEntry, want)
		}
		if len(candidates) > 1 {
			return nil, fmt.Errorf(i18n.UpgradeErrors.MultipleCount, len(candidates), want)
		}
		rc, err := candidates[0].Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		bin, err := io.ReadAll(io.LimitReader(rc, maxExtractedBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(bin)) > maxExtractedBytes {
			return nil, fmt.Errorf(i18n.UpgradeErrors.ExtractLimit, maxExtractedBytes)
		}
		if len(bin) == 0 {
			return nil, fmt.Errorf(i18n.UpgradeErrors.EmptyEntry, want)
		}
		return bin, nil
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	// tar 是顺序解压格式：跳过非目标条目也要消耗解压流量。给整个解压流
	// 加累计预算（limitReader），防高压缩率归档在目标条目前塞入巨型条目
	// 耗尽内存/CPU。zip 走中心目录随机访问，跳过条目不解压，无此问题。
	lr := &limitReader{r: gr, remaining: maxArchiveStreamBytes}
	tr := tar.NewReader(lr)
	var bin []byte
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf(i18n.UpgradeErrors.ReadArchive, err)
		}
		if filepath.Base(hdr.Name) != want || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if found {
			return nil, fmt.Errorf(i18n.UpgradeErrors.MultipleEntries, want)
		}
		found = true
		bin, err = io.ReadAll(io.LimitReader(tr, maxExtractedBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(bin)) > maxExtractedBytes {
			return nil, fmt.Errorf(i18n.UpgradeErrors.ExtractLimit, maxExtractedBytes)
		}
	}
	if !found {
		return nil, fmt.Errorf(i18n.UpgradeErrors.NoEntry, want)
	}
	if len(bin) == 0 {
		return nil, fmt.Errorf(i18n.UpgradeErrors.EmptyEntry, want)
	}
	return bin, nil
}

// replaceSelf 用临时文件原子替换当前二进制。
// Linux/macOS：rename 直接覆盖（运行中进程不受影响）；Windows：先把运行中的
// exe 改名为 .old（运行中文件不能删但可改名），移入新文件后延迟删除 .old。
func (u *upgrader) replaceSelf(bin []byte) error {
	dir := filepath.Dir(u.exePath)
	tmp, err := os.CreateTemp(dir, ".serialhub-new-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 替换成功后已不存在，失败时清理
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if u.goos != "windows" {
		return os.Rename(tmpName, u.exePath)
	}
	// 备份名带纳秒时间戳避免并发升级争用同一个 .old；
	// 未做升级互斥，两个升级进程同时执行仍可能互相干扰（罕见场景）。
	old := fmt.Sprintf("%s.old-%d", u.exePath, time.Now().UnixNano())
	if err := os.Rename(u.exePath, old); err != nil {
		return fmt.Errorf(i18n.UpgradeErrors.MoveOld, err)
	}
	if err := os.Rename(tmpName, u.exePath); err != nil {
		// 尽力恢复，避免 exe 缺位；恢复失败时报告备份位置让用户手动处理
		if rbErr := os.Rename(old, u.exePath); rbErr != nil {
			return fmt.Errorf(i18n.UpgradeErrors.InstallAndRollback, err, rbErr, old)
		}
		return err
	}
	// 延迟删除 .old（PowerShell 多轮重试；cmd /c 传脚本串与 Go 的
	// argv 转义不兼容，内部引号变 \" 后 cmd 无法正确解析）
	// 与 uninstall 自删共用同一段重试脚本（uninstall.go powershellRetryRemove）
	del := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
		powershellRetryRemove("o", old))
	if err := del.Start(); err != nil {
		fmt.Fprintf(os.Stderr, i18n.UpgradeErrors.KeepOld, old)
	}
	return nil
}

// trimVPrefix 去掉 tag 的 v 前缀。
func trimVPrefix(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

// compareVersion 比较点分版本号：a<b 返回 -1，相等 0，a>b 返回 1。
// 支持本项目发布 tag 的 vX.Y.Z（每段可带 -suffix 预发布后缀）格式；
// 不是完整 SemVer 实现（构建元数据 +build.x、多段预发布排序不保证）。
// 每段先按前导数值比较；数值相同再比后缀，无后缀（正式版）大于有后缀
// （预发布），如 0.5.1-beta < 0.5.1。
func compareVersion(a, b string) int {
	as := strings.Split(trimVPrefix(a), ".")
	bs := strings.Split(trimVPrefix(b), ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := "0", "0"
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if c := comparePart(av, bv); c != 0 {
			return c
		}
	}
	return 0
}

// comparePart 比较单个版本段。
func comparePart(a, b string) int {
	if an, aerr := strconv.Atoi(a); aerr == nil {
		if bn, berr := strconv.Atoi(b); berr == nil {
			switch {
			case an < bn:
				return -1
			case an > bn:
				return 1
			default:
				return 0
			}
		}
	}
	an, arem := splitLeadingDigits(a)
	bn, brem := splitLeadingDigits(b)
	if an != bn {
		if an < bn {
			return -1
		}
		return 1
	}
	// 前导数值相同：无后缀（正式版）更大
	switch {
	case arem == brem:
		return 0
	case arem == "":
		return 1
	case brem == "":
		return -1
	default:
		return strings.Compare(arem, brem)
	}
}

// splitLeadingDigits 拆出段的前导数值与剩余部分；无前导数字时数值返回 -1。
func splitLeadingDigits(s string) (int, string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return -1, s
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return -1, s
	}
	return n, s[i:]
}
