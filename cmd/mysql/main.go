package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type user struct {
	id   int
	age  int
	name string
}

var db *sql.DB

func initDB() (err error) {
	dsn := "root:mySQLDocker@tcp(127.0.0.1:3306)/sql_test?charset=utf8mb4&parseTime=True"
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return err
	}

	err = db.Ping() //检测dsn是否正确
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(50)
	db.SetConnMaxLifetime(time.Hour)

	return nil
}

func main() {
	err := initDB()
	if err != nil {
		fmt.Printf("init db error: %v\n", err)
		return
	}

	insertRowDemo() //插入数据

	//updateRowDemo() //更新数据

	//deleteRowDemo() //删除数据

	//queryRowDemo() //查询单行

	//queryMultiRowDemo() //查询多行

	transactionDemo() //事务
}

// 使用db.QueryRow查询单行数据
func queryRowDemo() {
	sqlStr := "SELECT id, name, age FROM user where id=?"

	var u user

	// QuryRow执行一次查询，并期望返回最多一行(Row)结果。总是返回非nil的值
	err := db.QueryRow(sqlStr, 1).Scan(&u.id, &u.name, &u.age)
	if err != nil {
		fmt.Printf("scan error: %v\n", err)
		return
	}

	fmt.Printf("id: %d, name: %s, age: %d\n", u.id, u.name, u.age)
}

// 使用db.Query查询多行数据
func queryMultiRowDemo() {
	sqlStr := "SELECT id, name, age FROM user where id > ?"

	rows, err := db.Query(sqlStr, 0)
	if err != nil {
		fmt.Printf("query error: %v\n", err)
		return
	}
	defer rows.Close() //非常重要：关闭rows释放持有的数据库链接

	//循环读取结果集中的数据
	for rows.Next() {
		var u user
		err := rows.Scan(&u.id, &u.name, &u.age)
		if err != nil {
			fmt.Printf("scan error: %v\n", err)
			return
		}

		fmt.Printf("id:%d name:%s age:%d\n", u.id, u.name, u.age)
	}
}

// 使用db.Exec插入数据
func insertRowDemo() {
	sqlStr := "INSERT INTO user(name, age) value (?,?)"
	ret, err := db.Exec(sqlStr, "王五", 38)
	if err != nil {
		fmt.Printf("insert error: %v\n", err)
		return
	}

	lastInsertId, err := ret.LastInsertId() //新插入数据的id
	if err != nil {
		fmt.Printf("get lastInsertID error: %v\n", err)
		return
	}

	fmt.Printf("insert success, last insert ID: %d\n", lastInsertId)
}

// 使用db.Exec()更新数据
func updateRowDemo() {
	sqlStr := "UPDATE user SET age=? WHERE name=?"
	ret, err := db.Exec(sqlStr, 31, "王五")
	if err != nil {
		fmt.Printf("update error: %v\n", err)
		return
	}

	n, err := ret.RowsAffected() //操作影响的行数
	if err != nil {
		fmt.Printf("get RowAffected error: %v\n", err)
		return
	}

	fmt.Printf("update success, affected rows: %d\n", n)
}

// 使用db.Exec()删除数据
func deleteRowDemo() {
	sqlStr := "DELETE FROM user where name = ?"
	ret, err := db.Exec(sqlStr, "王五")
	if err != nil {
		fmt.Printf("delete error: %v\n", err)
	}

	n, err := ret.RowsAffected() //操作影响的行数
	if err != nil {
		fmt.Printf("get RowAffected error: %v\n", err)
		return
	}

	fmt.Printf("delete success, affected rows: %d\n", n)
}

func transactionDemo() {
	tx, err := db.Begin() //开启事务
	if err != nil {
		if tx != nil {
			tx.Rollback() //出错回滚
		}
		fmt.Printf("begin trans error: %v\n", err)
		return
	}

	sqlStr1 := "UPDATE user set age=30 where name=?"
	ret1, err := tx.Exec(sqlStr1, "王五")
	if err != nil {
		tx.Rollback()
		fmt.Printf("exec sql1 error: %v\n", err)
		return
	}
	affRow1, err := ret1.RowsAffected()
	if err != nil {
		tx.Rollback() //出错回滚
		fmt.Printf("exec ret1.RowsAffected error: %v\n", err)
		return
	}

	sqlStr2 := "UPDATE user set age=40 where name=?"
	ret2, err := tx.Exec(sqlStr2, "王五")
	if err != nil {
		tx.Rollback()
		fmt.Printf("exec sql2 error: %v\n", err)
		return
	}
	affRow2, err := ret2.RowsAffected()
	if err != nil {
		tx.Rollback() //出错回滚
		fmt.Printf("exec ret1.RowsAffected error: %v\n", err)
		return
	}

	fmt.Printf("aff1: %d, aff2: %d\n", affRow1, affRow2)

	if affRow1 == 1 && affRow2 == 1 {
		_ = tx.Commit() //提交事务
		fmt.Println("exec trans success!")
	} else {
		tx.Rollback() //受影响行不符合预期,回滚
		fmt.Printf("不符合业务，事务回滚。。。")
	}
}
