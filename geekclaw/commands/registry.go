package commands

import "fmt"

// Registry 存储命令的规范集合。
type Registry struct {
	defs  []Definition
	index map[string]int
}

// NewRegistry 创建命令注册表，存储分发和可选平台注册适配器使用的规范命令集。
func NewRegistry(defs []Definition) *Registry {
	stored := make([]Definition, len(defs))
	copy(stored, defs)

	index := make(map[string]int, len(stored)*2)
	for i, def := range stored {
		registerCommandName(index, def.Name, i)
		for _, alias := range def.Aliases {
			registerCommandName(index, alias, i)
		}
	}

	return &Registry{defs: stored, index: index}
}

// Definitions 返回所有已注册的命令定义。
// 命令可用性是全局的，不再按频道区分。
func (r *Registry) Definitions() []Definition {
	out := make([]Definition, len(r.defs))
	copy(out, r.defs)
	return out
}

// Lookup 根据规范化的命令名称或别名查找命令定义。
func (r *Registry) Lookup(name string) (Definition, bool) {
	key := normalizeCommandName(name)
	if key == "" {
		return Definition{}, false
	}
	idx, ok := r.index[key]
	if !ok {
		return Definition{}, false
	}
	return r.defs[idx], true
}

// MergeDefinitions 将额外的定义追加到注册表中，跳过名称或别名与现有条目冲突的定义。
// 冲突以错误形式返回用于日志记录，但不会阻止其余定义的注册。
func (r *Registry) MergeDefinitions(extra []Definition) []error {
	var conflicts []error
	for _, def := range extra {
		key := normalizeCommandName(def.Name)
		if _, exists := r.index[key]; exists {
			conflicts = append(conflicts, fmt.Errorf("command %q: name conflicts with existing command", def.Name))
			continue
		}

		aliasConflict := false
		for _, alias := range def.Aliases {
			akey := normalizeCommandName(alias)
			if _, exists := r.index[akey]; exists {
				conflicts = append(conflicts, fmt.Errorf("command %q: alias %q conflicts with existing command", def.Name, alias))
				aliasConflict = true
				break
			}
		}
		if aliasConflict {
			continue
		}

		idx := len(r.defs)
		r.defs = append(r.defs, def)
		registerCommandName(r.index, def.Name, idx)
		for _, alias := range def.Aliases {
			registerCommandName(r.index, alias, idx)
		}
	}
	return conflicts
}

// registerCommandName 将命令名称注册到索引映射中。
func registerCommandName(index map[string]int, name string, defIndex int) {
	key := normalizeCommandName(name)
	if key == "" {
		return
	}
	if _, exists := index[key]; exists {
		return
	}
	index[key] = defIndex
}
