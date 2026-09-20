// Package cli 提供面板的维护命令（供 go-ui 菜单与运维脚本调用）：
//
//	gost-webui -set-password "admin 新密码"
//	gost-webui -set-listen ":8899"
//	gost-webui -set-base-path "/panel"
//	gost-webui -set-public-host "1.2.3.4"
//	gost-webui -show
//
// 说明：面板运行时数据库被占用，请先停服务再执行（go-ui 会自动处理）。
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"gost-webui/internal/config"
	"gost-webui/internal/store"
)

// Options 是维护命令参数（空字符串表示未设置该命令）。
type Options struct {
	Password   string // "user pass" 或 "pass"（只改密码）
	Listen     string
	BasePath   *string // 允许设置为空串（根路径）
	PublicHost string
	Show       bool
}

// Handled 表示是否有维护命令需要执行。
func (o Options) Handled() bool {
	return o.Password != "" || o.Listen != "" || o.BasePath != nil || o.PublicHost != "" || o.Show
}

// Run 执行维护命令。
func Run(cfgPath string, o Options) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	if o.Listen != "" {
		if err := setYAML(cfgPath, "", "listen", fmt.Sprintf("%q", o.Listen)); err != nil {
			return err
		}
		fmt.Printf("监听地址已设置为 %s\n", o.Listen)
	}
	if o.BasePath != nil {
		v := config.NormalizeBasePath(*o.BasePath)
		if err := setYAML(cfgPath, "", "base_path", fmt.Sprintf("%q", v)); err != nil {
			return err
		}
		if v == "" {
			fmt.Println("访问路径已设置为 根路径 /")
		} else {
			fmt.Printf("访问路径已设置为 %s/\n", v)
		}
	}
	if o.PublicHost != "" {
		if err := setYAML(cfgPath, "", "public_host", fmt.Sprintf("%q", o.PublicHost)); err != nil {
			return err
		}
		fmt.Printf("服务器地址已设置为 %s\n", o.PublicHost)
	}
	if o.Password != "" {
		user, pass := "admin", o.Password
		if f := strings.Fields(o.Password); len(f) == 2 {
			user, pass = f[0], f[1]
		}
		if len(pass) < 6 {
			return fmt.Errorf("密码至少 6 位")
		}
		if err := setYAML(cfgPath, "admin", "username", fmt.Sprintf("%q", user)); err != nil {
			return err
		}
		if err := setYAML(cfgPath, "admin", "password", fmt.Sprintf("%q", pass)); err != nil {
			return err
		}
		fmt.Printf("管理员账号已更新：%s\n", user)
	}

	// 同步数据库（数据库优先于配置文件，需要一并处理）
	if o.Listen != "" || o.BasePath != nil || o.PublicHost != "" || o.Password != "" {
		if err := syncDB(cfg, o); err != nil {
			return err
		}
	}

	if o.Show {
		printSummary(cfgPath, cfg)
	}
	return nil
}

func syncDB(cfg *config.Config, o Options) error {
	dbPath := filepath.Join(cfg.DataDir, "panel.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil // 数据库还没生成，无需同步
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败（面板可能仍在运行，请先停止服务）: %w", err)
	}
	defer st.Close()

	if o.Listen != "" {
		_ = st.SetSetting("panel_listen", o.Listen)
	}
	if o.BasePath != nil {
		_ = st.SetSetting("base_path", config.NormalizeBasePath(*o.BasePath))
	}
	if o.PublicHost != "" {
		_ = st.SetSetting("public_host", o.PublicHost)
	}
	if o.Password != "" {
		// 清除旧哈希并写入新哈希，立即生效且无需等首次登录
		_ = st.SetSetting("admin_hash", "")
		user := "admin"
		pass := o.Password
		if f := strings.Fields(o.Password); len(f) == 2 {
			user, pass = f[0], f[1]
		}
		_ = st.SetSetting("admin_user", user)
		if h, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost); err == nil {
			_ = st.SetSetting("admin_hash", string(h))
		}
	}
	return nil
}

func printSummary(cfgPath string, cfg *config.Config) {
	fmt.Println("当前配置:")
	fmt.Printf("  配置文件   : %s\n", cfgPath)
	fmt.Printf("  监听地址   : %s\n", cfg.Listen)
	base := cfg.BasePath
	if base == "" {
		base = "/"
	} else {
		base += "/"
	}
	fmt.Printf("  访问路径   : %s\n", base)
	host := cfg.PublicHost
	if host == "" {
		host = "(自动探测)"
	}
	fmt.Printf("  服务器地址 : %s\n", host)
	fmt.Printf("  管理员     : %s\n", cfg.Admin.Username)
	fmt.Printf("  数据目录   : %s\n", cfg.DataDir)
	fmt.Printf("  gost 二进制: %s\n", cfg.Gost.Bin)
}

// setYAML 就地修改 YAML 文件中的某个键（保留注释与其它内容）。
// section 为空表示顶层键；否则在 "section:" 段内查找。
func setYAML(path, section, key, value string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	secIndent := -1
	inSection := section == ""
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if section != "" {
			// 进入/离开目标段
			if indent == 0 && strings.HasSuffix(trimmed, ":") {
				name := strings.TrimSuffix(trimmed, ":")
				if name == section {
					inSection = true
					secIndent = indent
					continue
				} else if inSection {
					inSection = false
				}
			}
			if !inSection || indent <= secIndent {
				continue
			}
		} else if indent != 0 {
			continue
		}
		// 匹配 key:
		if strings.HasPrefix(trimmed, key+":") {
			newLine := fmt.Sprintf("%s%s: %s", strings.Repeat(" ", indent), key, value)
			lines[i] = newLine
			found = true
			break
		}
	}
	if !found {
		// 追加：顶层直接加在末尾；段内加在该段最后一行之后
		line := fmt.Sprintf("%s: %s", key, value)
		if section != "" {
			line = "  " + line
			insertAt := len(lines)
			for i, l := range lines {
				t := strings.TrimSpace(l)
				if t == section+":" {
					insertAt = i + 1
					for j := i + 1; j < len(lines); j++ {
						nt := strings.TrimSpace(lines[j])
						if nt != "" && !strings.HasPrefix(nt, "#") && len(lines[j])-len(strings.TrimLeft(lines[j], " ")) == 0 {
							break
						}
						insertAt = j + 1
					}
					break
				}
			}
			lines = append(lines[:insertAt], append([]string{line}, lines[insertAt:]...)...)
		} else {
			lines = append(lines, line)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}
