package dbx

import (
	"bytes"
	"database/sql"
	"dbweb/lib/mapfun"
	"dbweb/lib/safe"
	"dbweb/lib/tempext"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	PKValuesNumberError = fmt.Errorf("the pk values number error")
)

type SqlError struct {
	Sql    string
	Params interface{}
	Err    error
}

func (e SqlError) Error() string {
	l := 0
	content := fmt.Sprintf("%#v", e.Params)
	switch tv := e.Params.(type) {
	case []interface{}:
		l = len(tv)
	case map[string]interface{}:
		l = len(tv)
		list := []string{}
		for k, v := range tv {
			list = append(list, fmt.Sprintf("%s(%T):%#v", k, v, v))
		}
		content = strings.Join(list, "\n")
	}
	return fmt.Sprintf("%s\n[%s]\nparams len is %d,content is:\n%s", e.Err, e.Sql, l, content)
}

type DB interface {
	Select(dest interface{}, query string, args ...interface{}) error
	NamedQuery(query string, arg interface{}) (*sqlx.Rows, error)
	NamedExec(query string, arg interface{}) (sql.Result, error)
	PrepareNamed(query string) (*sqlx.NamedStmt, error)
	DriverName() string
	QueryRowx(query string, args ...interface{}) *sqlx.Row
	Queryx(query string, args ...interface{}) (*sqlx.Rows, error)
	Rebind(string) string
	BindNamed(string, interface{}) (string, []interface{}, error)
	Exec(string, ...interface{}) (sql.Result, error)
	MustExec(string, ...interface{}) sql.Result
	Get(dest interface{}, query string, args ...interface{}) error
}

func Columns(db DB, strSql string, pam map[string]interface{}) ([]string, error) {
	rows, err := db.NamedQuery(strSql, pam)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	r, err := rows.Columns()
	if err == nil {
		for i, v := range r {
			r[i] = strings.ToUpper(v)
		}
	}

	return r, err
}
func TableNames(db DB) (names []string) {
	var strSql string
	switch db.DriverName() {
	case "postgres":
		strSql = "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()"
	case "oci8":
		strSql = "SELECT table_name FROM user_tables"
	case "mysql":
		strSql = "SELECT table_name FROM information_schema.tables WHERE table_schema = schema()"
	default:
		panic("not impl," + db.DriverName())
	}
	names = []string{}
	if err := db.Select(&names, strSql); err != nil {
		panic(err)
	}
	for i, v := range names {
		names[i] = strings.ToUpper(v)
	}
	sort.Strings(names)
	return
}
func NameGet(db DB, d interface{}, strSql string, p map[string]interface{}) error {
	str, pam := BindSql(db, strSql, p)
	return db.Get(d, str, pam...)
}
func NameSelect(db DB, d interface{}, strSql string, p map[string]interface{}) error {
	str, pam := BindSql(db, strSql, p)
	return db.Select(d, str, pam...)
}
func QueryRecord(db DB, strSql string, p map[string]interface{}) (result []map[string]interface{}, err error) {
	var rows *sqlx.Rows
	str, pam := BindSql(db, strSql, p)
	if rows, err = db.Queryx(str, pam...); err != nil {
		err = SqlError{strSql, p, err}
		return
	}
	result = []map[string]interface{}{}
	defer rows.Close()
	for rows.Next() {
		oneRecord := map[string]interface{}{}
		if err = rows.MapScan(oneRecord); err != nil {
			err = SqlError{strSql, p, err}
			return
		}

		result = append(result, mapfun.UpperKeys(oneRecord))
	}
	if len(result) == 0 {
		log.Println(strSql, pam)
	}
	return
}
func IsNull(db DB) string {
	switch db.DriverName() {
	case "oci8":
		return "nvl"
	case "postgres":
		return "COALESCE"
	case "mysql", "sqlite3":
		return "ifnull"
	default:
		panic("not impl")

	}

}
func Exists(db DB, strSql string, p map[string]interface{}) (result bool, err error) {
	str, pam := BindSql(db, strSql, p)

	var rows *sqlx.Rows
	if rows, err = db.Queryx(str, pam...); err != nil {
		err = SqlError{strSql, p, err}
		return
	}
	defer rows.Close()
	result = rows.Next()
	return
}

//执行create table as select语句
func CreateTableAs(db DB, tableName, strSql string, pks []string) error {
	switch db.DriverName() {
	case "postgres", "mysql", "oci8":
		s := fmt.Sprintf("CREATE TABLE %s as %s", tableName, strSql)
		if _, err := db.Exec(s); err != nil {
			return SqlError{s, nil, err}
		}
		s = fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY(%s)", tableName, strings.Join(pks, ","))
		if _, err := db.Exec(s); err != nil {
			return SqlError{s, nil, err}
		}
	default:
		panic("not impl create table as")
	}
	return nil
}

//删除表字段
func TableRemoveColumns(db DB, tabName string, cols []string) error {
	var strSql string
	switch db.DriverName() {
	case "postgres", "mysql":
		strList := []string{}
		for _, v := range cols {
			strList = append(strList, "DROP COLUMN "+v)
		}
		strSql = fmt.Sprintf("ALTER table %s %s", tabName, strings.Join(strList, ","))
	case "oci8":
		strSql = fmt.Sprintf("ALTER table %s drop(%s)", tabName, strings.Join(cols, ","))
	default:
		return fmt.Errorf("not impl," + db.DriverName())
	}
	log.Println(strSql)
	if _, err := db.Exec(strSql); err != nil {
		return SqlError{strSql, nil, err}
	}
	return nil

}

//表更名
func TableRename(db DB, oldName, newName string) error {
	var strSql string
	switch db.DriverName() {
	case "postgres", "sqlite3":
		strSql = fmt.Sprintf("ALTER table %s RENAME TO %s", oldName, newName)
	case "oci8", "mysql":
		strSql = fmt.Sprintf("rename table %s TO %s", oldName, newName)
	default:
		return fmt.Errorf("not impl," + db.DriverName())
	}
	log.Println(strSql)
	if _, err := db.Exec(strSql); err != nil {
		return SqlError{strSql, nil, err}
	}
	return nil
}
func TableExists(db DB, tableName string) (bool, error) {
	var strSql string
	switch db.DriverName() {
	case "postgres":
		strSql = "SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() and upper(table_name)=:tname"
	case "oci8":
		strSql = "SELECT count(*) FROM user_tables where table_name=:tname"
	case "mysql":
		strSql = "SELECT count(*) FROM information_schema.tables WHERE table_schema = schema() and UPPER(table_name)=:tname"
	default:
		return false, fmt.Errorf("not impl," + db.DriverName())
	}
	var iCount int64
	p := map[string]interface{}{"tname": strings.ToUpper(tableName)}
	if err := NameGet(db, &iCount, strSql, p); err != nil {
		return false, SqlError{strSql, p, err}
	}
	return iCount > 0, nil
}
func GetSqlFun(db DB, strSql string, p map[string]interface{}) (result interface{}, err error) {
	str, pam := BindSql(db, strSql, p)

	var rows *sqlx.Rows
	if rows, err = db.Queryx(str, pam...); err != nil {
		err = SqlError{strSql, p, err}
		return
	}
	defer rows.Close()
	if rows.Next() {
		if err = rows.Scan(&result); err != nil {
			err = SqlError{strSql, p, err}
			return
		}
	}
	return
}

func MustGetSqlFun(db DB, strSql string, p map[string]interface{}) (result interface{}) {
	var err error
	if result, err = GetSqlFun(db, strSql, p); err != nil {
		log.Printf("sql err:%s\n%s\n", err, strSql)
		panic(err)
	}
	return
}
func MustQueryRecord(db DB, strSql string, p map[string]interface{}) (result []map[string]interface{}) {
	var err error
	if result, err = QueryRecord(db, strSql, p); err != nil {
		panic(err)
	}
	return
}
func MustRow(db DB, strSql string, p map[string]interface{}) map[string]interface{} {
	var err error
	result, err := QueryRecord(db, strSql, p)
	if err != nil {
		panic(err)
	}
	if len(result) == 0 {
		panic(SqlError{strSql, p, sql.ErrNoRows})
	}
	return result[0]
}

//获取一个临时表名
func GetTempTableName(db DB, prev string) (string, error) {
	//确定名称
	tableName := ""
	rand.Seed(time.Now().UnixNano())
	bys := make([]byte, 4)
	icount := 0
	for {
		binary.BigEndian.PutUint32(bys, rand.Uint32())
		tableName = fmt.Sprintf("%s%X", prev, bys)
		if exists, err := TableExists(db, tableName); err != nil {
			return "", err
		} else if !exists {
			break
		}
		icount++
		if icount > 100 {
			return "", fmt.Errorf("find table name too much")
		}
	}
	return tableName, nil
}

//批量执行，分号换行的会被分开执行
func BatchExec(db DB, strSql string, params map[string]interface{}) error {
	for _, v := range strings.Split(strSql, ";\n") {
		if len(strings.TrimSpace(v)) == 0 {
			continue
		}
		_, err := db.NamedExec(v, params)
		if err != nil {
			return err
		}
	}
	return nil
}

type RenderSqlError struct {
	Template   string
	SqlParam   map[string]interface{}
	RenderArgs interface{}
	Err        error
}

func (r *RenderSqlError) Error() string {
	return fmt.Sprintf("Template:\n%s\nSqlParam:\n%#v\nRenderArgs:\n%#v\nError:\n%s", r.Template, r.SqlParam, r.RenderArgs, r.Err)
}

//修改{{P}}的语法，因为后期的交叉汇总等需要sql传递的功能，生成参数就无法实现了，改成内嵌的字符串
//×渲染一个sql，可以用{{P val}}的语法加入一个参数，就不用考虑字符串转义了
//后期如果速度慢，可以加入一个模板缓存
func RenderSql(strSql string, renderArgs interface{}) string {

	if len(strSql) == 0 {
		return strSql
	}
	var err error
	var t *template.Template
	if t, err = template.New("sql").Funcs(tempext.GetFuncMap()).Parse(strSql); err != nil {
		panic(&RenderSqlError{strSql, nil, renderArgs, err})
	}

	out := bytes.NewBuffer(nil)
	if err = t.Execute(out, renderArgs); err != nil {
		panic(&RenderSqlError{strSql, nil, renderArgs, err})
	}
	strSql = out.String()
	return strSql
}
func Count(db DB, strSql string, params map[string]interface{}) (result int64, err error) {
	v, err := GetSqlFun(db, fmt.Sprintf("SELECT COUNT(*) FROM (%s) count_sql", strSql), params)
	if err != nil {
		return
	}
	result = safe.Int(v)
	return
}
func BindSqlWithError(db DB, strSql string, params map[string]interface{}) (result string, paramsValues []interface{}, err error) {
	//转换in的条件
	sql, pam, err := sqlx.Named(strSql, params)
	if err != nil {
		err = &SqlError{strSql, params, err}
		return
	}
	sql, pam, err = sqlx.In(sql, pam...)
	if err != nil {
		err = &SqlError{strSql, params, err}
		return
	}

	result = db.Rebind(sql)
	paramsValues = pam
	return
}
func BindSql(db DB, strSql string, params map[string]interface{}) (result string, paramsValues []interface{}) {
	//转换in的条件
	sql, pam, err := sqlx.Named(strSql, params)
	if err != nil {
		panic(&SqlError{strSql, params, err})
	}
	sql, pam, err = sqlx.In(sql, pam...)
	if err != nil {
		panic(&SqlError{strSql, params, err})
	}

	result = db.Rebind(sql)
	paramsValues = pam
	return
}

//新增单字段索引
func CreateColumnIndex(db DB, tableName, colName string) error {
	var strSql string
	switch db.DriverName() {
	case "postgres":
		strSql = fmt.Sprintf("create index on %s(%s)", tableName, colName)
	case "oci8", "mysql", "sqlite3":
		//这里会有问题，如果表名和字段名比较长就会出错
		strSql = fmt.Sprintf("create index idx_%s_%s on %s(%s)", tableName, colName, tableName, colName)
	default:
		panic("not impl " + db.DriverName())
	}
	if _, err := db.Exec(strSql); err != nil {
		return SqlError{strSql, nil, err}
	}
	log.Println(strSql)
	return nil
}

//删除单字段索引
func DropColumnIndex(db DB, tableName, indexName string) error {
	var strSql string

	switch db.DriverName() {
	case "postgres", "oci8", "sqlite3":
		strSql = fmt.Sprintf("drop index %s", indexName)
	case "mysql":
		strSql = fmt.Sprintf("drop index %s on %s", indexName, tableName)
	default:
		panic("not impl," + db.DriverName())
	}
	if _, err := db.Exec(strSql); err != nil {
		return SqlError{strSql, nil, err}
	}
	return nil
}

//新增主键
func AddTablePrimaryKey(db DB, tableName string, pks []string) error {
	var strSql string

	switch db.DriverName() {
	case "postgres", "mysql":
		strSql = fmt.Sprintf("alter table %s add primary key(%s)", tableName, strings.Join(pks, ","))
	case "oci8":
		strSql = fmt.Sprintf("alter table %s add constraint %s_pk primary key(%s)", tableName, tableName, strings.Join(pks, ","))
	default:
		panic("not impl," + db.DriverName())
	}
	if _, err := db.Exec(strSql); err != nil {
		return SqlError{strSql, nil, err}
	}
	return nil
}

//删除主键
func DropTablePrimaryKey(db DB, tableName string) error {
	switch db.DriverName() {
	case "postgres":
		//先获取主键索引的名称，然后删除索引
		strSql := fmt.Sprintf(
			"select b.relname from  pg_index a inner join pg_class b on a.indexrelid =b.oid where indisprimary and indrelid='%s'::regclass",
			tableName)
		pkCons := ""
		if err := db.Get(&pkCons, strSql); err != nil {
			return SqlError{strSql, nil, err}
		}
		strSql = fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", tableName, pkCons)
		if _, err := db.Exec(strSql); err != nil {
			return SqlError{strSql, nil, err}
		}
	case "oci8":
		strSql := fmt.Sprintf(
			"select constraint_name from user_CONSTRAINTS where table_name ='%s' and constraint_type='P'",
			strings.ToUpper(tableName))
		pkCons := ""
		if rows, err := QueryRecord(db, strSql, nil); err != nil {
			return SqlError{strSql, nil, err}
		} else {
			if len(rows) > 0 {
				pkCons = rows[0]["CONSTRAINT_NAME"].(string)
			} else {
				return nil
			}
		}
		strSql = fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", tableName, pkCons)
		if _, err := db.Exec(strSql); err != nil {
			return SqlError{strSql, nil, err}
		}
	case "mysql":
		strSql := fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY", tableName)
		if _, err := db.Exec(strSql); err != nil {
			return SqlError{strSql, nil, err}
		}
	default:
		panic("not impl," + db.DriverName())
	}
	return nil
}

//返回一个字段值的字符串表达式
func ValueExpress(db DB, dataType int, value string) string {
	switch dataType {
	case TypeFloat, TypeInt:
		return value
	case TypeString:
		return safe.SignString(value)
	case TypeDatetime:
		switch db.DriverName() {
		case "oci8":
			if len(value) == 10 {
				return fmt.Sprintf("to_date(%s,'yyyy-mm-dd')", safe.SignString(value))
			} else if len(value) == 19 {
				return fmt.Sprintf("to_date(%s,'yyyy-mm-dd hh24:mi:ss')", safe.SignString(value))
			} else {
				panic(fmt.Errorf("invalid datetime:%s", value))
			}
		default:
			panic(fmt.Errorf("not impl datetime,dbtype:%s", db.DriverName()))
		}
	default:
		panic(fmt.Errorf("not impl ValueExpress,type:%d", dataType))
	}
}
func MustExec(db DB, strSql string, params ...interface{}) {
	if _, err := db.Exec(strSql, params...); err != nil {
		panic(SqlError{strSql, params, err})
	}
	return
}
