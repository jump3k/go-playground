package main

import "fmt"

type Person struct {
	Name  string //姓名
	Birth string //生日
	ID    int64  //身份证号
}

// 指针类型，可以修改个人信息。因为是传递指针
func (person *Person) changeName(name string) {
	person.Name = name
}

// 非指针类型，打印个人信息
func (person Person) printMess() {
	fmt.Printf("name: %s, birthday: %s, ID: %d\n", person.Name, person.Birth, person.ID)

	person.ID = 3 //TODO: 不会更改传入peroson的值，因为非指针类型，接收器接收的值是原始值的副本
}

func main() {
	p1 := Person{
		Name:  "王二小",
		Birth: "1996-12-23",
		ID:    1,
	}

	p1.printMess()
	p1.changeName("王老小")
	p1.printMess()
}
