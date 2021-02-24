package main

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
)

type HashFunc func(data []byte) uint32

type CHash struct {
	hashRing     []uint32          // 哈希环
	hashFunc     HashFunc          // 哈希算法
	vNodeMapNode map[uint32]string // <虚拟结点，物理结点>

	hashRingSize int // 哈希环上虚拟结点个数
}

func NewCHash(hash HashFunc) *CHash {
	if hash == nil {
		hash = crc32.ChecksumIEEE
	}

	ch := &CHash{
		hashRing:     make([]uint32, 0, 1000),
		hashFunc:     hash,
		vNodeMapNode: make(map[uint32]string),
		hashRingSize: 0,
	}

	return ch
}

func (ch *CHash) Add(node string, vNodes int) {
	for i := 0; i < vNodes; i++ {
		vNodeName := node + "_" + strconv.Itoa(i) // 虚拟结点
		key := ch.hashFunc([]byte(vNodeName))     // 虚拟结点哈希值
		ch.vNodeMapNode[key] = node               // 虚拟结点映射物理结点
		ch.hashRing = append(ch.hashRing, key)    // 加入哈希环
		ch.hashRingSize++
	}

	sort.Slice(ch.hashRing, func(i, j int) bool { return ch.hashRing[i] < ch.hashRing[j] })

	if sort.SliceIsSorted(ch.hashRing, func(i, j int) bool { return ch.hashRing[i] < ch.hashRing[j] }) {
		fmt.Println(ch.hashRing)
	}
}

func (ch *CHash) Get(input string) string {
	key := ch.hashFunc([]byte(input))
	idx := sort.Search(ch.hashRingSize, func(i int) bool { return ch.hashRing[i] >= key })
	if idx == ch.hashRingSize {
		idx = 0
	}

	node := ch.vNodeMapNode[ch.hashRing[idx]]

	return node
}

func main() {
	ch := NewCHash(nil)

	ch.Add("1.1.1.1", 5)
	ch.Add("2.2.2.2", 5)
	ch.Add("3.3.3.3", 5)

	for i := 0; i < 30; i++ {
		input := strconv.Itoa(i) + "_" + "Shanghai"
		node := ch.Get(input)
		fmt.Println(node)
	}
}
