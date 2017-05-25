package mysql

import (
	"dbweb/lib/safe"
	"fmt"
	"sort"
	"strings"

	"github.com/linlexing/dbx"
)

type meta struct {
}

func (m *meta) IsNull() string {
	return "COALESCE"
}

func init() {
	dbx.RegisterMeta("postgres", new(meta))
}

//执行create table as select语句
func (m *meta) CreateTableAs(db dbx.DB, tableName, strSQL string, pks []string) error {
	s := fmt.Sprintf("CREATE TABLE %s as %s", tableName, strSQL)
	if _, err := db.Exec(s); err != nil {
		return dbx.NewSQLError(s, nil, err)
	}
	s = fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY(%s)", tableName, strings.Join(pks, ","))
	if _, err := db.Exec(s); err != nil {
		return dbx.NewSQLError(s, nil, err)
	}
	return nil
}

//CreateColumnIndex 新增单字段索引
func (m *meta) CreateColumnIndex(db dbx.DB, tableName, colName string) error {
	//这里会有问题，如果表名和字段名比较长就会出错
	strSQL := fmt.Sprintf("create index on %s(%s)", tableName, colName)
	if _, err := db.Exec(strSQL); err != nil {
		return dbx.NewSQLError(strSQL, nil, err)
	}
	return nil
}

func (m *meta) TableNames(db dbx.DB) (names []string, err error) {
	strSQL := "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()"
	names = []string{}
	if err = db.Select(&names, strSQL); err != nil {
		return
	}
	sort.Strings(names)
	return
}
func (m *meta) RemoveColumns(db dbx.DB, tabName string, cols []string) error {
	var strSQL string
	strList := []string{}
	for _, v := range cols {
		strList = append(strList, "DROP COLUMN "+v)
	}
	strSQL = fmt.Sprintf("ALTER table %s %s", tabName, strings.Join(strList, ","))
	if _, err := db.Exec(strSQL); err != nil {
		return dbx.NewSQLError(strSQL, nil, err)
	}
	return nil
}

func (m *meta) TableExists(db dbx.DB, tabName string) (bool, error) {
	schema := ""
	ns := strings.Split(tabName, ".")
	tname := ""
	if len(ns) > 1 {
		schema = ns[0]
		tname = ns[1]
	} else {
		tname = tabName
	}
	if len(schema) == 0 {
		schema = safe.String(dbx.MustGetSQLFun(db, "select current_schema()", nil))
	}

	strSQL := fmt.Sprintf(
		"SELECT count(*) FROM information_schema.tables WHERE table_schema ilike '%s' and table_name ilike :tname", schema)
	var iCount int64
	p := map[string]interface{}{"tname": strings.ToUpper(tname)}
	if err := dbx.NameGet(db, &iCount, strSQL, p); err != nil {
		return false, dbx.NewSQLError(strSQL, p, err)
	}
	return iCount > 0, nil
}
func (m *meta) TableRename(db dbx.DB, oldName, newName string) error {
	strSQL := fmt.Sprintf("ALTER table %s RENAME TO %s", oldName, newName)
	if _, err := db.Exec(strSQL); err != nil {
		return dbx.NewSQLError(strSQL, nil, err)
	}
	return nil
}
func (m *meta) DropColumnIndex(db dbx.DB, tableName, indexName string) error {
	strSQL := fmt.Sprintf("drop index %s", indexName)
	if _, err := db.Exec(strSQL); err != nil {
		return dbx.NewSQLError(strSQL, nil, err)
	}
	return nil
}