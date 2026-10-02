package mcpsetup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/dongly/serialhub/internal/i18n"
)

// ErrNoConfig 表示目标配置文件不存在——「存在才写」策略下不创建新文件。
var ErrNoConfig = errors.New("config file not found")

// NoConfigError 报告未找到配置文件，并列出探测过的候选路径。
type NoConfigError struct {
	Paths []string
}

func (e *NoConfigError) Error() string {
	return fmt.Sprintf(i18n.MCPSetupInstall.NoConfig, strings.Join(e.Paths, ", "))
}

// Is 支持 errors.Is(err, ErrNoConfig) 判定。
func (e *NoConfigError) Is(target error) bool { return target == ErrNoConfig }

// configCandidates 按优先级返回候选配置路径：
// OpenCode 同目录可能并存 opencode.json 与 opencode.jsonc（jsonc 后加载、优先级更高，
// 也是 OpenCode 自身的默认新建选择），优先写入 jsonc；其他客户端仅原路径。
func configCandidates(path string) []string {
	if filepath.Base(path) == "opencode.json" {
		return []string{filepath.Join(filepath.Dir(path), "opencode.jsonc"), path}
	}
	return []string{path}
}

// requireConfigFile 按「存在才写」策略检查 path（含空白判定与符号链接解析），
// 缺失时返回携带探测路径的 NoConfigError，供 CLI 型配置面复用同一跳过语义。
func requireConfigFile(path string) error {
	_, _, ok, err := findConfig(path)
	if err != nil {
		return err
	}
	if !ok {
		return &NoConfigError{Paths: configCandidates(path)}
	}
	return nil
}

// foundConfig 是一个探测到的非空白配置文件。
type foundConfig struct {
	path string
	raw  []byte
}

// findConfigAll 返回全部存在的非空白候选（OpenCode json/jsonc 并存时两个都返回），
// 供卸载与迁移清理遍历；顺序与 configCandidates 一致（jsonc 优先）。
// 符号链接解析到真实目标（dotfiles 管理，不拆链接）；内容为空白视为「无配置」、
// 文件不存在同样跳过；其他读取错误（权限、目录等）如实返回。
func findConfigAll(path string) ([]foundConfig, error) {
	var out []foundConfig
	seen := map[string]bool{} // 两候选经符号链接指向同一文件时按真实路径去重
	for _, cand := range configCandidates(path) {
		real := resolveSymlinkPath(cand)
		if seen[real] {
			continue
		}
		seen[real] = true
		b, rerr := os.ReadFile(real)
		if rerr != nil {
			if os.IsNotExist(rerr) {
				continue
			}
			return nil, fmt.Errorf(i18n.MCPSetupInstall.ReadFile, real, rerr)
		}
		if len(bytes.TrimSpace(b)) == 0 {
			continue
		}
		out = append(out, foundConfig{path: real, raw: b})
	}
	return out, nil
}

// findConfig 返回首个存在的非空白候选（写入目标：OpenCode 并存时写 jsonc），
// 全部未命中时 ok=false，由调用方按「存在才写」策略处理。
func findConfig(path string) (real string, raw []byte, ok bool, err error) {
	all, err := findConfigAll(path)
	if err != nil || len(all) == 0 {
		return "", nil, false, err
	}
	return all[0].path, all[0].raw, true, nil
}

// jsonIndentUnit 探测原文件的缩进单位（首个缩进行的前导空白），未探测到时用两空格。
func jsonIndentUnit(raw []byte) string {
	for _, line := range strings.Split(string(raw), "\n") {
		i := 0
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i > 0 && i < len(line) {
			return line[:i]
		}
	}
	return "  "
}

// ensureContainer 沿点分 topKey 定位容器对象，缺失的中间层补为空对象成员
// （成员缩进 unit*(i+1)，闭合缩进与之对齐）；已存在但不是对象时返回 NotAnObject 错误，
// root 非对象返回 RootNotObject 错误。返回的 *hujson.Object 可直接原地增删成员。
func ensureContainer(root *hujson.Value, topKey, unit string) (*hujson.Object, error) {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New(i18n.MCPSetupInstall.RootNotObject)
	}
	parts := strings.Split(topKey, ".")
	for i, part := range parts {
		idx := findMember(obj, part)
		if idx < 0 {
			appendMember(obj, hujson.ObjectMember{
				Name: hujson.Value{
					BeforeExtra: hujson.Extra("\n" + strings.Repeat(unit, i+1)),
					Value:       hujson.Literal(`"` + part + `"`),
				},
				Value: hujson.Value{BeforeExtra: hujson.Extra(" "), Value: &hujson.Object{}},
			}, unit, i+1)
			idx = len(obj.Members) - 1
		}
		child := &obj.Members[idx].Value
		next, ok := child.Value.(*hujson.Object)
		if !ok {
			return nil, fmt.Errorf(i18n.MCPSetupInstall.NotAnObject, strings.Join(parts[:i+1], "."))
		}
		obj = next
	}
	return obj, nil
}

// appendMember 向 obj 追加成员并保证容器闭合大括号落在独立行：
// obj 原 AfterExtra 含换行（正常格式化文件）则原样保留（注释不动）；
// 纯空白无换行（如 `{}`、`} `）则替换为「换行 + 容器闭合缩进」；
// 含注释无换行则在其后补换行与闭合缩进。
// depth 是成员所在层级（root 的成员 depth=1），容器闭合缩进为 unit*(depth-1)。
func appendMember(obj *hujson.Object, m hujson.ObjectMember, unit string, depth int) {
	obj.Members = append(obj.Members, m)
	if bytes.ContainsRune(obj.AfterExtra, '\n') {
		return
	}
	if len(bytes.TrimSpace(obj.AfterExtra)) == 0 {
		obj.AfterExtra = []byte("\n" + strings.Repeat(unit, depth-1))
		return
	}
	obj.AfterExtra = append(append([]byte{}, obj.AfterExtra...), '\n')
	obj.AfterExtra = append(obj.AfterExtra, strings.Repeat(unit, depth-1)...)
}

// findMember 返回名为 name 的成员下标，不存在或名字非字符串字面量时返回 -1。
func findMember(obj *hujson.Object, name string) int {
	for i := range obj.Members {
		if s, ok := memberName(&obj.Members[i].Name); ok && s == name {
			return i
		}
	}
	return -1
}

// memberName 解出成员名（hujson 中名字是字符串字面量）。
func memberName(v *hujson.Value) (string, bool) {
	lit, ok := v.Value.(hujson.Literal)
	if !ok {
		return "", false
	}
	s, err := strconv.Unquote(string(lit))
	if err != nil {
		return "", false
	}
	return s, true
}

// setEntry 把 entry 写入容器 obj 下的 name 成员：缺失则追加（成员缩进
// unit*(depth-1)、值内层缩进 unit、前缀与成员缩进对齐），已存在则仅替换值，
// 保留原位置的 BeforeExtra/AfterExtra（邻近注释与空白不动）。
// depth 是成员所在层级（root 的成员 depth=1）。
func setEntry(obj *hujson.Object, name string, entry map[string]any, unit string, depth int) error {
	indent := strings.Repeat(unit, depth)
	b, err := json.MarshalIndent(entry, indent, unit)
	if err != nil {
		return err
	}
	if idx := findMember(obj, name); idx >= 0 {
		obj.Members[idx].Value.Value = hujson.Literal(string(b))
		return nil
	}
	appendMember(obj, hujson.ObjectMember{
		Name: hujson.Value{
			BeforeExtra: hujson.Extra("\n" + indent),
			Value:       hujson.Literal(`"` + name + `"`),
		},
		Value: hujson.Value{BeforeExtra: hujson.Extra(" "), Value: hujson.Literal(string(b))},
	}, unit, depth)
	return nil
}

// removeEntry 删除 topKey.name 成员并自深向浅剪掉空容器（root 保留）。
// 路径未命中（层级缺失或中间非对象）或条目不存在时返回 false，不改动树。
func removeEntry(root *hujson.Value, topKey, name string) bool {
	obj, ok := root.Value.(*hujson.Object)
	if !ok {
		return false
	}
	parts := strings.Split(topKey, ".")
	chain := []*hujson.Object{obj}
	for _, part := range parts {
		idx := findMember(obj, part)
		if idx < 0 {
			return false
		}
		next, ok := obj.Members[idx].Value.Value.(*hujson.Object)
		if !ok {
			return false
		}
		obj = next
		chain = append(chain, obj)
	}
	idx := findMember(obj, name)
	if idx < 0 {
		return false
	}
	obj.Members = append(obj.Members[:idx], obj.Members[idx+1:]...)
	// 自深向浅剪空容器；chain[i] 对应 parts[i-1]，root（chain[0]）不删。
	for i := len(chain) - 1; i >= 1; i-- {
		if len(chain[i].Members) != 0 {
			break
		}
		parent := chain[i-1]
		pi := findMember(parent, parts[i-1])
		if pi < 0 {
			break
		}
		parent.Members = append(parent.Members[:pi], parent.Members[pi+1:]...)
	}
	return true
}

// packWithNewline 序列化并保证以换行结尾（与既有写回格式一致）。
func packWithNewline(v *hujson.Value) []byte {
	out := v.Pack()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out
}
