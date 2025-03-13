package kv

import (
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/kmlixh/consulTool/errors"
)

// KV 提供 Consul KV 操作的功能
type KV struct {
	client *api.Client
}

// NewKV 创建新的 KV 操作实例
func NewKV(client *api.Client) *KV {
	return &KV{
		client: client,
	}
}

// Get 获取指定键的值
func (k *KV) Get(key string) (*api.KVPair, error) {
	if key == "" {
		return nil, errors.ErrEmptyKey
	}

	pair, _, err := k.client.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get KV: %w", err)
	}

	if pair == nil {
		return nil, errors.ErrKeyNotFound
	}

	return pair, nil
}

// GetWithPrefix 获取指定前缀的所有键值对
func (k *KV) GetWithPrefix(prefix string) (api.KVPairs, error) {
	if prefix == "" {
		return nil, errors.ErrEmptyKey
	}

	pairs, _, err := k.client.KV().List(prefix, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list KVs with prefix %s: %w", prefix, err)
	}

	if len(pairs) == 0 {
		return nil, errors.ErrKeyNotFound
	}

	return pairs, nil
}

// Put 设置键值对
func (k *KV) Put(key string, value []byte) error {
	if key == "" {
		return errors.ErrEmptyKey
	}

	p := &api.KVPair{
		Key:   key,
		Value: value,
	}

	_, err := k.client.KV().Put(p, nil)
	if err != nil {
		return fmt.Errorf("failed to put KV: %w", err)
	}

	return nil
}

// Delete 删除键值对
func (k *KV) Delete(key string) error {
	if key == "" {
		return errors.ErrEmptyKey
	}

	_, err := k.client.KV().Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV: %w", err)
	}

	return nil
}

// DeleteWithPrefix 删除指定前缀的所有键值对
func (k *KV) DeleteWithPrefix(prefix string) error {
	if prefix == "" {
		return errors.ErrEmptyKey
	}

	_, err := k.client.KV().DeleteTree(prefix, nil)
	if err != nil {
		return fmt.Errorf("failed to delete KV tree with prefix %s: %w", prefix, err)
	}

	return nil
}
