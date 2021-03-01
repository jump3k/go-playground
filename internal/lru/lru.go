package lru

import "container/list"

type Key interface{}

// 链表结点
type entry struct {
	key   Key
	value interface{}
}

type Cache struct {
	MaxEntries int                              // 缓存容量
	ll         *list.List                       // 链表
	cache      map[Key]*list.Element            // 键值对映射表
	OnEvicted  func(key Key, value interface{}) // 当删除Key时回调
}

func New(maxEntries int) *Cache {
	return &Cache{
		MaxEntries: maxEntries,
		ll:         list.New(),
		cache:      make(map[Key]*list.Element),
	}
}

func (c *Cache) Add(key Key, value interface{}) {
	if c.cache == nil {
		c.cache = make(map[Key]*list.Element)
		c.ll = list.New()
	}

	if ee, ok := c.cache[key]; ok { //命中
		c.ll.MoveToFront(ee)            //移至链表头部位置
		ee.Value.(*entry).value = value //重新赋值
		return
	}

	// 未命中
	ele := c.ll.PushFront(&entry{key, value}) //新建entry
	c.cache[key] = ele                        //保存至cache

	if c.MaxEntries != 0 && c.ll.Len() > c.MaxEntries {
		c.RemoveOldest() // 移除链表末尾结点
	}
}

func (c *Cache) Get(key Key) (value interface{}, ok bool) {
	if c.cache == nil {
		return
	}

	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele) // 移至链表头部位置
		return ele.Value.(*entry).value, true
	}

	return
}

func (c *Cache) Remove(key Key) {
	if c.cache == nil {
		return
	}

	if ele, hit := c.cache[key]; hit {
		c.removeElement(ele)
	}
}

func (c *Cache) Len() int {
	if c.cache == nil {
		return 0
	}

	return c.ll.Len()
}

func (c *Cache) Clear() {
	if c.OnEvicted != nil {
		for _, e := range c.cache {
			kv := e.Value.(*entry)
			c.OnEvicted(kv.key, kv.value)
		}
	}

	c.ll = nil
	c.cache = nil
}

func (c *Cache) RemoveOldest() {
	if c.cache == nil {
		return
	}

	ele := c.ll.Back() //末尾结点
	if ele != nil {
		c.removeElement(ele)
	}
}

func (c *Cache) removeElement(e *list.Element) {
	c.ll.Remove(e) //删除链表结点

	kv := e.Value.(*entry)
	delete(c.cache, kv.key) // 从键值对映射表中删除

	if c.OnEvicted != nil {
		c.OnEvicted(kv.key, kv.value) // 回调
	}
}
