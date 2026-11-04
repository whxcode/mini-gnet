package tools

import "os/exec"

// Info 是本地工具菜单的数据；执行信息不进入发送给模型的 Definition。
type Info struct {
	Definition Definition
	Launch     LaunchInfo
}

// LaunchInfo 描述启动配置，不代表工具已经执行。动态参数用占位符并在 Note 中解释。
type LaunchInfo struct {
	Command        string
	Args           []string
	ExecutablePath string
	WorkDir        string
	ToolDir        string
	ConfigPath     string
	Note           string
}

// LaunchDescriber 允许工具提供本地启动说明，不强制纯 Go 工具实现进程相关信息。
type LaunchDescriber interface {
	DescribeLaunch(Context) LaunchInfo
}

// Infos 按注册顺序读取菜单数据，只解析程序路径，不启动任何工具。
func (r *Registry) Infos(ctx Context) []Info {
	var items []Info
	if r == nil {
		return items
	}

	for _, name := range r.order {
		entry := r.entries[name]
		info := Info{Definition: entry.definition.Clone()}

		if describer, ok := entry.tool.(LaunchDescriber); ok {
			info.Launch = describer.DescribeLaunch(ctx)
			info.Launch.Args = append([]string(nil), info.Launch.Args...)

			path, err := exec.LookPath(info.Launch.Command)
			if err == nil {
				info.Launch.ExecutablePath = path
			}
		}

		items = append(items, info)
	}

	return items
}
