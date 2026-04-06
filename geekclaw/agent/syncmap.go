package agent

import "sync"

// TypedMap 是 sync.Map 的类型安全封装，
// 消除了调用处的运行时类型断言。
type TypedMap[K comparable, V any] struct {
	m sync.Map
}

// Load 根据键加载值，如果键不存在或类型不匹配则返回零值和 false。
func (tm *TypedMap[K, V]) Load(key K) (V, bool) {
	v, ok := tm.m.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	typed, ok := v.(V)
	if !ok {
		var zero V
		return zero, false
	}
	return typed, true
}

// Store 存储键值对。
func (tm *TypedMap[K, V]) Store(key K, value V) {
	tm.m.Store(key, value)
}

// Delete 删除指定键。
func (tm *TypedMap[K, V]) Delete(key K) {
	tm.m.Delete(key)
}

// LoadOrStore 如果键存在则返回已有值，否则存储并返回给定值。
// 如果已有值类型不匹配，存储新值并返回。
func (tm *TypedMap[K, V]) LoadOrStore(key K, value V) (V, bool) {
	actual, loaded := tm.m.LoadOrStore(key, value)
	typed, ok := actual.(V)
	if !ok {
		// 类型不匹配（不应发生），用新值覆盖
		tm.m.Store(key, value)
		return value, false
	}
	return typed, loaded
}
