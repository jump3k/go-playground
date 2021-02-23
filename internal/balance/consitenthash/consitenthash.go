package consitenthash

import (
	"errors"
	"hash/crc32"
	"sort"
	"strconv"
)

type HashFunc func(data []byte) uint32

type ConsistentHashLoadBalance struct {
	hashFunc     HashFunc          // 哈希算法
	hashRing     []uint32          // 哈希环
	hashRingSize int               // 哈希环上虚拟节点总的个数
	nodeMapNum   map[string]int    // <物理节点，虚拟结点个数>
	vNodeMapNode map[uint32]string // <虚拟key, 物理节点>
}

func NewConsistentHash(hash HashFunc) *ConsistentHashLoadBalance {
	if hash == nil {
		hash = crc32.ChecksumIEEE // 哈希算法默认使用crc32.ChecksumIEEE
	}

	return &ConsistentHashLoadBalance{
		hashFunc:     hash,
		hashRing:     []uint32{},
		hashRingSize: 0,
		nodeMapNum:   make(map[string]int),
		vNodeMapNode: make(map[uint32]string),
	}
}

func (ch *ConsistentHashLoadBalance) Add(params ...string) error {
	if len(params) < 2 {
		return errors.New("params at least 2 elements")
	}

	node := params[0]
	vnodeNum, err := strconv.Atoi(params[1])
	if err != nil {
		return errors.New("invalid vnode_num input")
	}

	ch.nodeMapNum[node] = vnodeNum // 将物理结点和它的虚拟结点个数做映射

	for i := 0; i < vnodeNum; i++ {
		name := node + "_" + strconv.Itoa(i)   // 虚拟结点格式：node_%d
		key := ch.hashFunc([]byte(name))       // 虚拟结点计算哈希值
		ch.vNodeMapNode[key] = node            // 将虚拟结点哈希值和物理结点做映射
		ch.hashRing = append(ch.hashRing, key) // 将虚拟结点哈希值放置哈希环上
		ch.hashRingSize++
	}

	//sort.Sort(ch.hashRing) // 排序哈希环上的虚拟结点
	sort.Slice(ch.hashRing, func(i, j int) bool { return ch.hashRing[i] < ch.hashRing[j] })

	return nil
}

func (ch *ConsistentHashLoadBalance) Delete(params ...string) error {
	node := params[0]

	if _, ok := ch.nodeMapNum[node]; !ok {
		return errors.New("node not exist")
	}

	vnodeNum := ch.nodeMapNum[node] // 该物理结点对应的虚拟结点个数
	for i := 0; i < vnodeNum; i++ {
		name := node + "_" + strconv.Itoa(i) // 虚拟结点格式：node_%d
		key := ch.hashFunc([]byte(name))     // 虚拟结点计算哈希值
		for idx, v := range ch.hashRing {
			if v == key {
				ch.hashRing = append(ch.hashRing[:idx], ch.hashRing[idx+1:]...) // 删除idx处元素
			}
		}
		delete(ch.vNodeMapNode, key)
		ch.hashRingSize--
	}
	delete(ch.nodeMapNum, node)

	return nil
}

func (ch *ConsistentHashLoadBalance) Get(params ...string) (string, error) {
	key := ch.hashFunc([]byte(params[0]))
	idx := sort.Search(ch.hashRingSize, func(i int) bool { return ch.hashRing[i] >= key })
	if idx == ch.hashRingSize {
		idx = 0
	}

	hitKey := ch.hashRing[idx]      // 命中的虚拟结点
	node := ch.vNodeMapNode[hitKey] // 找到虚拟结点对应的物理结点

	//fmt.Printf("input: %s, key: %d, idx: %d, vnode: %d", params[0], key, idx, hitKey)

	return node, nil
}

func (ch *ConsistentHashLoadBalance) GetHashRingSize() int {
	return ch.hashRingSize
}
