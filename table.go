package dbx

import (
	"bytes"
	"database/sql"
	"dbweb/lib/safe"

	"encoding/base64"
	"encoding/gob"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/linlexing/mapfun"

	"github.com/jmoiron/sqlx"
)

const (
	TypeString = iota
	TypeInt
	TypeDatetime
	TypeBytea
	TypeFloat
)

type DBTableColumn struct {
	Name        string `db:"DBNAME"`
	Type        string `db:"DBTYPE"`
	MaxLength   int    `db:"DBMAXLENGTH"`
	Null        bool   `db:"DBNULL"`
	TrueType    string `db:"TRUETYPE"`
	FetchDriver string //上次获取字段信息时，数据库驱动的名称

	Index      bool     `db:"-"`
	IndexName  string   `db:"-"` //如果该字段有索引，存放数据库中索引的名称
	FormerName []string `db:"-"`
}
type ColumnType struct {
	Name string
	Type string
}

//判定两个字段定义是否相等
func (field *DBTableColumn) Eque(src *DBTableColumn) bool {
	return field.Name == src.Name &&
		field.Type == src.Type &&
		(field.MaxLength == src.MaxLength ||
			field.MaxLength <= 0 && src.MaxLength <= 0) &&
		field.Null == src.Null
}
func (f *DBTableColumn) ToJson(v interface{}) (interface{}, error) {
	if v == nil {
		return nil, nil
	}
	switch f.GoType() {
	case TypeBytea: //base64
		return base64.RawStdEncoding.EncodeToString(safe.Bytea(v)), nil
	case TypeDatetime: //RFC3339
		if tv, ok := v.(time.Time); ok {
			return tv.Format(time.RFC3339), nil
		} else {
			return nil, fmt.Errorf("the column %s value %v not is time", f.Name, v)
		}
	case TypeFloat:
		if tv, ok := v.(float64); ok {
			return tv, nil
		} else {
			return nil, fmt.Errorf("the column %s value %#v not is float64", f.Name, v)
		}
	case TypeInt:
		if tv, ok := v.(int64); ok {
			return tv, nil
		} else {
			return nil, fmt.Errorf("the column %s value %v not is int64", f.Name, v)
		}
	case TypeString:
		return safe.String(v), nil
	default:
		return nil, fmt.Errorf("the column %s type error", f.Name)
	}
}

func (f *DBTableColumn) FromJson(v interface{}) (interface{}, error) {
	if v == nil {
		return nil, nil
	}
	switch f.GoType() {
	case TypeBytea: //base64
		if tv, ok := v.(string); ok {
			return base64.RawStdEncoding.DecodeString(tv)
		} else {
			return nil, fmt.Errorf("the column %s json value %v not is base64 string", f.Name, v)
		}
		return base64.RawStdEncoding.EncodeToString(safe.Bytea(v)), nil
	case TypeDatetime: //RFC3339
		if tv, ok := v.(string); ok {
			return safe.StrToDate(tv)
		} else {
			return nil, fmt.Errorf("the column %s json value %v not is time string", f.Name, v)
		}
	case TypeFloat:
		if tv, ok := v.(float64); ok {
			return tv, nil
		} else {
			return nil, fmt.Errorf("the column %s json value %v not is float64", f.Name, v)
		}
	case TypeInt:
		switch tv := v.(type) {
		case string:
			return strconv.ParseInt(tv, 10, 64)
		case int:
			return int64(tv), nil
		case int64:
			return tv, nil
		case float32:
			return int64(tv), nil
		case float64:
			return int64(tv), nil
		default:
			return nil, fmt.Errorf("the column %s json value %v (%T) not is int64 string", f.Name, v, v)
		}
	case TypeString:
		if tv, ok := v.(string); ok {
			return tv, nil
		} else {
			return nil, fmt.Errorf("the column %s json value %v not is string", f.Name, v)
		}
	default:
		return nil, fmt.Errorf("the column %s type error", f.Name)
	}
}
func (field *DBTableColumn) ConvertToTrueType(v interface{}) (result interface{}) {
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{
				"column": field.Name,
				"type":   field.Type,
			}).Error(r)
			log.Panic(r)
		}
	}()

	//nil代表null，不需要转换，否则会出错
	if v == nil {
		return nil
	}
	//空字符串当null处理
	if v == "" {
		return nil
	}
	switch field.GoType() {
	case TypeString:
		switch tv := v.(type) {
		case []byte:
			result = string(tv)
		default:
			result = v
		}
	case TypeDatetime:
		switch tv := v.(type) {
		case time.Time:
			result = tv
		case nil:
			result = tv
		case string:
			result = safe.Date(tv)
		case []byte:
			result = safe.Date(string(tv))
		default:
			log.Panic(fmt.Errorf("error type,%T", v))
		}
	case TypeInt:
		//如果是nil会出错，所以在本函数的开头加了判断
		result = safe.Int(v)
	case TypeBytea:
		result = safe.Bytea(v)
	case TypeFloat:
		result = safe.Float64(v)
	default:
		result = v
	}

	return
}
func (c *DBTableColumn) ChineseType() string {
	switch c.Type {
	case "STR":
		return "字符串"
	case "INT":
		return "整型"
	case "DATE":
		return "日期"
	case "FLOAT":
		return "浮点"
	case "BYTEA":
		return "二进制"
	default:
		log.Panic("invalid type:" + c.Type)
		return ""
	}
}
func (c *DBTableColumn) GoType() int {
	switch c.Type {
	case "STR":
		return TypeString
	case "INT":
		return TypeInt
	case "DATE":
		return TypeDatetime
	case "FLOAT":
		return TypeFloat
	case "BYTEA":
		return TypeBytea
	default:
		log.Panic("invalid type:" + c.Type)
		return -1
	}
}
func ParseGoType(t int) string {
	switch t {
	case TypeString:
		return "STR"
	case TypeInt:
		return "INT"
	case TypeDatetime:
		return "DATE"
	case TypeFloat:
		return "FLOAT"
	case TypeBytea:
		return "BYTEA"
	default:
		log.Panic(fmt.Sprintf("invalid type:%d", t))
		return ""
	}

}
func (c *DBTableColumn) Clone() *DBTableColumn {
	return &DBTableColumn{c.Name, c.Type, c.MaxLength, c.Null, c.TrueType, c.FetchDriver, c.Index, c.IndexName, c.FormerName}
}

//postgres修改字段，不需要名称和notnull
func (c *DBTableColumn) DBType(driver string) string {
	if c.FetchDriver == driver && len(c.TrueType) > 0 {
		return c.TrueType
	}
	var dataType string
	switch driver {
	case "postgres":
		switch c.GoType() {
		case TypeBytea:
			dataType = "bytea"
		case TypeDatetime:
			dataType = "timestamp without time zone"
		case TypeFloat:
			dataType = "double precision"
		case TypeInt:
			dataType = "integer"
		case TypeString:
			if c.MaxLength <= 0 {
				dataType = "text"
			} else {
				dataType = fmt.Sprintf("character varying(%d)", c.MaxLength)
			}
		default:
			log.Panic("not impl DBType")

		}
	case "oci8":
		switch c.GoType() {
		case TypeBytea:
			dataType = "BLOB"
		case TypeDatetime:
			dataType = "DATE"
		case TypeFloat:
			dataType = "BINARY_DOUBLE"
		case TypeInt:
			dataType = "INT"
		case TypeString:
			if c.MaxLength <= 0 {
				dataType = "CLOB"
			} else {
				if c.MaxLength > 4000 {
					dataType = "VARCHAR2(4000)"
				} else {
					dataType = fmt.Sprintf("VARCHAR2(%d CHAR)", c.MaxLength)
				}

			}
		default:
			log.Panic("not impl DBType")

		}
	case "sqlite3":
		switch c.GoType() {
		case TypeBytea:
			dataType = "BLOB"
		case TypeDatetime:
			dataType = "DATE"
		case TypeFloat:
			dataType = "REAL"
		case TypeInt:
			dataType = "INTEGER"
		case TypeString:
			if c.MaxLength <= 0 {
				dataType = "TEXT"
			} else {
				dataType = fmt.Sprintf("TEXT(%d)", c.MaxLength)
			}
		default:
			log.Panic("not impl DBType")
		}
	case "mysql":
		switch c.GoType() {
		case TypeBytea:
			dataType = "BLOB"
		case TypeDatetime:
			dataType = "DATETIME"
		case TypeFloat:
			dataType = "DOUBLE PRECISION"
		case TypeInt:
			dataType = "BIGINT"
		case TypeString:
			if c.MaxLength <= 0 {
				dataType = "TEXT"
			} else {
				dataType = fmt.Sprintf("VARCHAR(%d)", c.MaxLength)
			}
		default:
			log.Panic("not impl DBType")
		}

	default:
		log.Panic("not impl DBType")
	}
	return dataType
}
func (c *DBTableColumn) DBDefine(driver string) string {
	nullStr := ""
	if !c.Null {
		nullStr = " NOT NULL"
	}
	return fmt.Sprintf("%s %s%s", c.Name, c.DBType(driver), nullStr)
}

//如果是null，则有null字样
func (c *DBTableColumn) DBDefineNull(driver string) string {
	nullStr := " NULL"
	if !c.Null {
		nullStr = " NOT NULL"
	}
	return fmt.Sprintf("%s %s%s", c.Name, c.DBType(driver), nullStr)
}
func (c *DBTableColumn) GoValue(v string) interface{} {
	if len(v) == 0 {
		return nil
	}
	switch c.GoType() {
	case TypeString:
		return v
	case TypeInt:
		if i, err := strconv.ParseInt(v, 10, 64); err != nil {
			log.Panic(err)
			return nil
		} else {
			return i
		}
	case TypeDatetime:
		t, err := time.Parse("2006-01-02 15:04:05", v)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05", v)
		}
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", v)
		}
		if err != nil {
			t, err = time.Parse(time.RFC3339, v)
		}
		if err != nil {
			t, err = time.Parse(time.RFC3339Nano, v)
		}

		if err != nil {
			t, err = time.Parse("2006-01-02", v)
		}
		if err != nil {
			log.Panic(err)
			return -1
		}
		return t
	case TypeBytea:
		return []byte(v)
	case TypeFloat:
		if f, err := strconv.ParseFloat(v, 64); err != nil {
			log.Panic(err)
			return nil
		} else {
			return f
		}
	default:
		log.Panic("not impl gotype", c)
		return nil
	}
}

type DBTable struct {
	Db             DB
	TableName      string
	Schema         string //对应数据库中方案的名称
	FormerName     []string
	primaryKeys    []string
	columns        []*DBTableColumn
	notnullColumns []string
	columnsNames   []string
	columnsMap     map[string]*DBTableColumn //用于快速查询
}

func NewTable(db DB, tabName string) *DBTable {
	if len(tabName) == 0 {
		log.Panic("table name is empty")
	}
	ns := strings.Split(tabName, ".")
	rev := &DBTable{
		Db: db,
	}
	if len(ns) > 1 {
		rev.Schema = strings.ToUpper(ns[0])
		rev.TableName = strings.ToUpper(ns[1])
	} else {
		rev.TableName = strings.ToUpper(tabName)
	}
	return rev
}
func (t *DBTable) Name() string {
	if len(t.Schema) > 0 {
		return t.Schema + "." + t.TableName
	} else {
		return t.TableName
	}
}
func (t *DBTable) PrimaryKeys() []string {
	if t.primaryKeys != nil {
		return t.primaryKeys
	}
	result := []string{}
	schema := t.Schema

	switch t.Db.DriverName() {
	case "postgres":

		if err := t.Db.Select(&result,
			`SELECT a.attname
			FROM   pg_index i
			JOIN   pg_attribute a ON a.attrelid = i.indrelid
			        AND a.attnum = ANY(i.indkey)
			WHERE  i.indrelid = $1::regclass
			AND    i.indisprimary;`, t.Name()); err != nil {
			log.Panic(err)
		}
	case "oci8":
		if len(schema) == 0 {
			schema = safe.String(MustGetSqlFun(t.Db, "select user from dual", nil))
		}
		if err := t.Db.Select(&result, fmt.Sprintf(
			`SELECT cols.column_name
			FROM all_constraints cons,all_cons_columns cols
			WHERE cons.owner='%s'
			and cons.OWNER=cols.owner
			and cols.table_name = :tblname
			AND cons.constraint_type = 'P'
			AND cons.constraint_name = cols.constraint_name
			AND cons.owner = cols.owner
			ORDER BY cols.table_name, cols.position`, schema), t.TableName); err != nil {
			log.Panic(err)
		}
	case "sqlite3":
		strSql := fmt.Sprintf(`PRAGMA table_info(%s)`, t.Name())
		r, _, err := QueryRecord(t.Db, strSql, nil)
		if err != nil {
			log.Panic(err)
		}
		for _, row := range r {
			if safe.Int(row["PK"]) > 0 {
				result = append(result, safe.String(row["NAME"]))
			}
		}
	case "mysql":

		strSql := fmt.Sprintf("SHOW KEYS FROM %s WHERE Key_name = 'PRIMARY'", t.Name())
		rows, _, err := QueryRecord(t.Db, strSql, nil)
		if err != nil {
			log.Panic(err)
		}
		for _, row := range rows {
			result = append(result, safe.String(row["COLUMN_NAME"]))
		}
	default:
		log.Panic(fmt.Errorf("not impl"))
	}
	for i, v := range result {
		result[i] = strings.ToUpper(v)
	}
	t.primaryKeys = result
	return result
}
func (t *DBTable) Columns() (result []string) {
	if t.columnsNames == nil {
		cols := t.AllField()
		result = make([]string, len(cols))
		for i, v := range cols {
			result[i] = v.Name
		}
		t.columnsNames = result
	}
	return t.columnsNames
}
func (t *DBTable) NotNullColumns() []string {
	if t.notnullColumns == nil {
		t.notnullColumns = []string{}
		for _, v := range t.AllField() {
			if !v.Null {
				t.notnullColumns = append(t.notnullColumns, v.Name)
			}
		}
	}
	return t.notnullColumns
}

//return nil if the record not found
func (t *DBTable) Row(pks ...interface{}) map[string]interface{} {
	pkNames := t.PrimaryKeys()
	if len(pkNames) != len(pks) {
		log.Panic(fmt.Errorf("the table %s pk values number error.table pk:%#v,pkvalues:%#v", t.Name(), pkNames, pks))
	}
	query := map[string]interface{}{}
	for i, v := range pks {
		query[pkNames[i]] = v
	}
	rows, err := t.Rows(query)
	if err != nil {
		log.Panic(err)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

//将一行数据转换成json,日期、二进制、int64数据转换成文本
func (t *DBTable) ToJsonRow(row map[string]interface{}) (map[string]interface{}, error) {
	transRecord := map[string]interface{}{}
	for k, v := range row {
		k = strings.ToUpper(k)
		if field := t.Field(k); field != nil {
			if sv, err := field.ToJson(v); err != nil {
				return nil, err
			} else {
				transRecord[k] = sv
			}
		} else {
			return nil, fmt.Errorf("can't find the column %s to json", k)
		}
	}
	return transRecord, nil
}

//将一个json数据转换回row
func (t *DBTable) FromJsonRow(row map[string]interface{}) (map[string]interface{}, error) {
	transRecord := map[string]interface{}{}

	for k, v := range row {
		k = strings.ToUpper(k)
		if field := t.Field(k); field != nil {
			if sv, err := field.FromJson(v); err != nil {
				return nil, err
			} else {
				transRecord[k] = sv
			}
		} else {
			return nil, fmt.Errorf("can't find the column %s at fromjson", k)
		}
	}
	return transRecord, nil
}

//将一行数据转换成实际的数据类型，根据字段名从表中查出类型
//同时将字段名转换成大写
func (t *DBTable) ConvertToTrueType(row map[string]interface{}) map[string]interface{} {
	transRecord := map[string]interface{}{}
	for k, v := range row {
		k = strings.ToUpper(k)
		if field := t.Field(k); field != nil {
			transRecord[k] = field.ConvertToTrueType(v)
		} else {
			switch tv := v.(type) {
			case []byte:
				transRecord[k] = string(tv)
			default:
				transRecord[k] = tv
			}
		}
	}
	return transRecord
}

//查询返回排序记录，返回记录字段名是大写，且数据类型正确转换
func (t *DBTable) QueryRowsOrder(where string, param map[string]interface{}, orderby []string, columns ...string) (record []map[string]interface{}, err error) {
	//使字段信息先收集，防止后面多个游标造成内存问题
	t.AllField()
	if len(where) > 0 {
		where = " where " + where
	}
	columnsStr := "*"
	if len(columns) > 0 {
		columnsStr = strings.Join(columns, ",")
	}
	str_orderby := ""
	if len(orderby) > 0 {
		str_orderby = " order by " + strings.Join(orderby, ",")
	}
	strSql := fmt.Sprintf("select %s from %s%s%s", columnsStr, t.Name(), where, str_orderby)
	var rows *sqlx.Rows
	rows, err = t.Db.NamedQuery(strSql, param)
	if err != nil {
		err = SqlError{strSql, param, err}
		return
	}
	record = []map[string]interface{}{}
	defer rows.Close()
	for rows.Next() {
		oneRecord := map[string]interface{}{}
		if err = rows.MapScan(oneRecord); err != nil {
			return
		}
		record = append(record, t.ConvertToTrueType(oneRecord))
	}
	return
}

//查询返回记录，返回记录字段名是大写，且数据类型正确转换
func (t *DBTable) QueryRows(where string, param map[string]interface{}, columns ...string) (record []map[string]interface{}, err error) {
	return t.QueryRowsOrder(where, param, nil, columns...)
}

//检查一个主键是否存在
func (t *DBTable) KeyExists(pks ...interface{}) (result bool, err error) {
	return t.Exists(mapfun.Object(t.PrimaryKeys(), pks))
}

//是否有记录
func (t *DBTable) Exists(query map[string]interface{}) (result bool, err error) {
	strWhere := []string{}

	newQuery := map[string]interface{}{}
	icount := 0
	for k, v := range query {
		pname := fmt.Sprintf("p%d", icount)
		icount++
		strWhere = append(strWhere, fmt.Sprintf("%s=:%s", k, pname))
		newQuery[pname] = v
	}
	where := ""
	if len(strWhere) > 0 {
		where = " where " + strings.Join(strWhere, " and ")
	}
	var rows *sqlx.Rows
	strSql := fmt.Sprintf("select 1 from %s%s", t.Name(), where)
	rows, err = t.Db.NamedQuery(strSql, newQuery)
	if err != nil {
		err = SqlError{strSql, newQuery, err}
		return
	}
	defer rows.Close()
	result = rows.Next()
	return
}
func (t *DBTable) KeyValues(row map[string]interface{}) []interface{} {
	return mapfun.Values(mapfun.Pick(row, t.PrimaryKeys()...))
}

//统计记录数
//参数可以传入string,map[string]interface{}
func (t *DBTable) MustCount(params ...interface{}) int64 {
	i, err := t.Count(params...)
	if err != nil {
		log.Panic(err)
		return -1

	}
	return i

}
func (t *DBTable) Count(params ...interface{}) (int64, error) {
	var strSql string
	var pam map[string]interface{}
	strSql = "select count(*) from " + t.Name()
	if len(params) > 0 && len(strings.TrimSpace(params[0].(string))) > 0 {

		strSql = fmt.Sprintf("select count(*) from %s where %s", t.Name(), params[0].(string))
	}
	if len(params) > 1 {
		pam = params[1].(map[string]interface{})
	}
	if len(params) > 2 {
		log.Panic("error number params")
	}
	r, err := GetSqlFun(t.Db, strSql, pam)
	if err != nil {
		return -1, err
	}
	return safe.Int(r), nil
}

//返回的字段名称是大写的字母
func (t *DBTable) Rows(query map[string]interface{}, columns ...string) (record []map[string]interface{}, err error) {
	strWhere := []string{}
	//name query 不允许用汉字做参数名，需要转换
	newQuery := map[string]interface{}{}
	icount := 0
	for k, v := range query {
		pname := fmt.Sprintf("p%d", icount)
		icount++

		strWhere = append(strWhere, fmt.Sprintf("%s=:%s", k, pname))
		newQuery[pname] = v
	}
	return t.QueryRows(strings.Join(strWhere, " and "), newQuery, columns...)
}
func (t *DBTable) checkNotNull(row map[string]interface{}) error {
	for _, v := range t.NotNullColumns() {
		if val, ok := row[v]; ok && val == nil {
			return fmt.Errorf("the not null column:%s is null", v)
		}
	}
	return nil
}

//检查row中是否含有非空字段的值，以及去掉多余的字段值
//如果是oracle，则需要去除时间中的时区，以免触发ORA-01878错误
func (t *DBTable) checkAndConvertRow(row map[string]interface{}) (map[string]interface{}, error) {
	rev := mapfun.Pick(row, t.Columns()...)
	if t.Db.DriverName() == "oci8" {
		for k, v := range rev {
			if t.Field(k).Type == "DATE" && v != nil {
				rev[k] = safe.TruncateTimeZone(safe.Date(v))
			}
		}
	}
	if err := t.checkNotNull(rev); err != nil {
		return nil, err
	}
	return rev, nil
}

//从一个sql语句导入数据,sql语句返回的列必须与表中数量一致
func (t *DBTable) CreateAs(dataDB DB, strSql string,
	typeTableName string, typeColumns []*ColumnType, uniqueField []string,
	progressFunc func(string)) (err error) {
	rowCount, err := Count(dataDB, strSql, nil)
	if err != nil {
		log.Println(err)
		return
	}

	progressFunc(fmt.Sprintf("start CreateAs table %s,total %d records", t.Name(), rowCount))
	tabDB := t.Db.(*sqlx.DB)
	rows, err := dataDB.Queryx(strSql)
	if err != nil {
		log.Println(err)
		return
	}
	defer rows.Close()
	typeTable := NewTable(dataDB, typeTableName)
	cols, err := rows.Columns()
	if err != nil {
		log.Println(err)
		return
	}
	values := make([]interface{}, len(cols))
	for i := range values {
		values[i] = new(interface{})
	}
	//创建表结构
	colsIndex := map[string]bool{}
	colsDef := []*DBTableColumn{}
	pkDef := []string{}
	for i, v := range cols {
		cols[i] = strings.ToUpper(v)
		colsIndex[cols[i]] = true
		colDef := &DBTableColumn{
			Name:      cols[i],
			Type:      "STR",
			Null:      true,
			MaxLength: -1,
		}
		if field := typeTable.Field(cols[i]); field != nil {
			colDef = field.Clone()
		} else {
			for _, field1 := range typeColumns {
				if field1.Name == cols[i] {
					colDef.Type = field1.Type
					break
				}
			}
		}
		colsDef = append(colsDef, colDef)
	}
	//检查唯一字段是否在导出中，以确定主键
	bContain := true
	for _, v := range uniqueField {
		if _, ok := colsIndex[v]; !ok {
			bContain = false
			break
		}
	}
	if bContain {
		pkDef = uniqueField
	}
	t.Define(colsDef, pkDef)
	if err = t.Create(); err != nil {
		log.Println(err)
		return
	}
	var icount, batCount int64 = 0, 0

	//再构造insert语句
	insertSql := fmt.Sprintf(
		"insert into %s(%s)values(%s)",
		t.Name(), strings.Join(cols, ","),
		strings.Join(strings.Split(strings.Repeat("?", len(cols)), ""), ","))
	insertSql = t.Db.Rebind(insertSql)
	//再开始事务
	startTime := time.Now()
	beginTime := startTime
	tx, err := tabDB.Beginx()
	if err != nil {
		log.Println(err)
		return
	}
	insertStmt, err := tx.Prepare(insertSql)
	if err != nil {
		log.Println(err)
		return
	}
	icount = 0
	batCount = 0
	for rows.Next() {
		if err = rows.Scan(values...); err != nil {
			log.Println(err)
			tx.Rollback()
			return
		}
		vs := make([]interface{}, len(cols))
		for i, v := range values {
			vs[i] = t.AllField()[i].ConvertToTrueType(*(v.(*interface{})))
		}
		if _, err = insertStmt.Exec(vs...); err != nil {
			log.Printf("error:%s,values:\n", err)
			for ei, ev := range vs {
				log.Printf("\t%s=%#v", cols[ei], ev)
			}
			tx.Rollback()
			return
		}
		icount++
		batCount++
		totalSec := time.Since(startTime).Seconds()
		if totalSec >= 5 {
			if err = insertStmt.Close(); err != nil {
				log.Println(err)
				return
			}
			if err = tx.Commit(); err != nil {
				log.Println(err)
				return
			}
			batCount = 0
			tx, err = tabDB.Beginx()
			if err != nil {
				log.Println(err)
				return
			}
			insertStmt, err = tx.Prepare(insertSql)
			if err != nil {
				log.Println(err)
				return
			}
			progressFunc(fmt.Sprintf("\t%.2f%%\t%d/%d\t%.2fs", 100.0*float64(icount)/float64(rowCount), icount, rowCount, totalSec))
			startTime = time.Now()
		}
	}
	if batCount > 0 {
		if err = insertStmt.Close(); err != nil {
			log.Println(err)
			tx.Rollback()
			return
		}
		if err = tx.Commit(); err != nil {
			log.Println(err)
			return
		}
	}
	if err = rows.Err(); err != nil {
		log.Println(err)
		return
	}
	progressFunc(fmt.Sprintf("%s,total %d records imported %.2fs", t.Name(), icount, time.Since(beginTime).Seconds()))

	return

}

//生成一个InsertStmt
func (t *DBTable) InsertStmt() (stmt *sqlx.NamedStmt, colMap map[string]string, err error) {
	columns := []string{}
	pColumns := []string{}
	colMap = map[string]string{}
	icount := 0
	for _, field := range t.Columns() {
		//字段名转换成大写
		columns = append(columns, field)
		pname := fmt.Sprintf("p%d", icount)
		icount++
		pColumns = append(pColumns, ":"+pname)
		colMap[field] = pname
	}
	strSql := fmt.Sprintf(
		"insert into %s(%s)values(%s)",
		t.Name(), strings.Join(columns, ","),
		strings.Join(pColumns, ","))
	if stmt, err = t.Db.PrepareNamed(strSql); err != nil {
		err = SqlError{strSql, nil, err}
	}
	return
}

//仅非空字段生成语句
func (t *DBTable) insertAsPack(row map[string]interface{}) (err error) {
	columns := []string{}
	pColumns := []string{}
	icount := 0
	param := map[string]interface{}{}
	mapfun.Pack(row)
	for k, v := range row {
		//字段名转换成大写
		columns = append(columns, strings.ToUpper(k))
		pname := fmt.Sprintf("p%d", icount)
		param[pname] = v
		icount++
		pColumns = append(pColumns, ":"+pname)
	}
	strSql := fmt.Sprintf(
		"insert into %s(%s)values(%s)",
		t.Name(), strings.Join(columns, ","),
		strings.Join(pColumns, ","))
	if _, err = t.Db.NamedExec(strSql, param); err != nil {
		return SqlError{strSql, param, err}
	}
	return
}

//编码key值，如果是复合主键，则用gob序列化
func (t *DBTable) EncodeKey(keys ...interface{}) []byte {
	if len(keys) == 1 {
		switch tv := keys[0].(type) {
		case string:
			return []byte(tv)
		case []byte:
			return tv
		}
	}
	out := bytes.NewBuffer(nil)
	if err := gob.NewEncoder(out).Encode(keys); err != nil {
		log.Panic(err)
	}
	return out.Bytes()

}

//解开主键
func (t *DBTable) DecodeKey(key []byte) []interface{} {
	keycols := t.PrimaryKeys()
	if len(keycols) == 1 {
		switch t.Field(keycols[0]).GoType() {
		case TypeBytea:
			return []interface{}{key}
		case TypeString:
			return []interface{}{string(key)}
		}
	}
	in := bytes.NewBuffer(key)
	rev := []interface{}{}
	if err := gob.NewDecoder(in).Decode(&rev); err != nil {
		log.Panic(err)
	}
	return rev
}

//插入一批记录,使用第一行数据中的字段，并没有使用表中的字段
func (t *DBTable) Insert(rows []map[string]interface{}) (err error) {
	if len(rows) == 1 {
		if one, e := t.checkAndConvertRow(rows[0]); e != nil {
			err = e
			return
		} else {
			return t.insertAsPack(one)
		}
	}
	//先检查并转换数据
	data := []map[string]interface{}{}
	for _, row := range rows {
		if one, e := t.checkAndConvertRow(row); e != nil {
			err = e
			return
		} else {
			data = append(data, one)
		}
	}
	var stmt *sqlx.NamedStmt
	if data == nil || len(data) == 0 {
		return
	}
	columns := []string{}
	pColumns := []string{}
	pColumnMap := map[string]string{}
	icount := 0
	for k, _ := range data[0] {
		//字段名转换成大写
		columns = append(columns, strings.ToUpper(k))
		pname := fmt.Sprintf("p%d", icount)
		icount++
		pColumns = append(pColumns, ":"+pname)
		pColumnMap[k] = pname
	}
	strSql := fmt.Sprintf(
		"insert into %s(%s)values(%s)",
		t.Name(), strings.Join(columns, ","),
		strings.Join(pColumns, ","))
	if stmt, err = t.Db.PrepareNamed(strSql); err != nil {
		err = SqlError{strSql, nil, err}
		return
	}
	defer stmt.Close()
	for _, one := range data {
		newData := map[string]interface{}{}
		for k, v := range one {
			newData[pColumnMap[strings.ToUpper(k)]] = v
		}
		if _, err = stmt.Exec(newData); err != nil {
			return
		}

	}
	return
}

//删除记录，全部字段值将被生成where字句(text除外)
func (t *DBTable) Delete(rows []map[string]interface{}) (err error) {
	//考虑到null值，所有的行不能用一个语句，必须单独删除
	for _, v := range rows {
		if err = t.Remove(v); err != nil {
			return
		}
	}
	return
}

func (t *DBTable) RemoveByKeyValues(keyValues ...interface{}) (err error) {
	return t.RemoveByQuery(mapfun.Object(t.PrimaryKeys(), keyValues))
}
func (t *DBTable) RemoveByQuery(query map[string]interface{}) (err error) {
	param := map[string]interface{}{}
	pcount := 0
	where := []string{}
	for keyName, keyValue := range query {
		pname := fmt.Sprintf("p%d", pcount)
		where = append(where, fmt.Sprintf("%s=:%s", keyName, pname))
		param[pname] = keyValue
	}
	strSql := fmt.Sprintf("delete from %s where %s", t.Name(), strings.Join(where, " and "))

	var sqlr sql.Result
	if sqlr, err = t.Db.NamedExec(strSql, param); err != nil {
		err = SqlError{strSql, param, err}
		return
	}
	var rowAff int64
	if rowAff, err = sqlr.RowsAffected(); err != nil {
		return
	}
	if rowAff == 0 {
		return sql.ErrNoRows
		//return fmt.Errorf("the record not found,query:%v", query)
	}
	return
}

//删除一个记录，必须是全指标的记录
func (t *DBTable) Remove(row map[string]interface{}) (err error) {
	row, err = t.checkAndConvertRow(row)
	if err != nil {
		return
	}
	icount := 0
	strWhere := []string{}
	newRow := map[string]interface{}{}
	for k, v := range row {
		icount++
		pname := fmt.Sprintf("p%d", icount)
		//如果是没有长度的string，即text，以及bytea则不参与where条件
		if fld := t.Field(k); fld.GoType() != TypeBytea && (fld.GoType() != TypeString || fld.MaxLength > 0) {

			if v == nil {
				strWhere = append(strWhere, fmt.Sprintf("%s is null", k))
			} else {
				strWhere = append(strWhere, fmt.Sprintf("%s=:%s", k, pname))
				newRow[pname] = v
			}
		}
	}
	strSql := fmt.Sprintf(
		"delete from %s where %s", t.Name(), strings.Join(strWhere, " and "))
	var sqlr sql.Result
	if sqlr, err = t.Db.NamedExec(strSql, newRow); err != nil {
		err = SqlError{strSql, newRow, err}
		return
	}
	var rowAff int64
	if rowAff, err = sqlr.RowsAffected(); err != nil {
		return
	}
	if rowAff == 0 {
		return fmt.Errorf("the record not found,row:%v", row)
	}

	return
}

//通过一个key更新记录
func (t *DBTable) UpdateByKey(key []interface{}, row map[string]interface{}) (err error) {
	query := map[string]interface{}{}
	for i, v := range t.PrimaryKeys() {
		query[v] = key[i]
	}
	return t.UpdateByQuery(query, row)
}

//通过一个条件更新指定的字段值
func (t *DBTable) UpdateByQuery(query map[string]interface{}, row map[string]interface{}) (err error) {
	if len(row) == 0 {
		log.Panic(fmt.Errorf("data is null,row:%v,query:%v", row, query))
	}

	if err = t.checkNotNull(row); err != nil {
		return err
	}
	param := map[string]interface{}{}
	pcount := 0
	where := []string{}
	for k, v := range query {
		if v == nil {
			where = append(where, fmt.Sprintf("%s is null", k))
		} else {
			pname := fmt.Sprintf("p%d", pcount)
			where = append(where, fmt.Sprintf("%s=:%s", k, pname))
			param[pname] = v
			pcount++
		}
	}
	set := []string{}
	for k, v := range row {
		pname := fmt.Sprintf("p%d", pcount)
		set = append(set, fmt.Sprintf("%s=:%s", k, pname))
		param[pname] = v
		pcount++
	}
	whereStr := ""
	if len(where) > 0 {
		whereStr = "where " + strings.Join(where, " and ")
	}
	strSql := fmt.Sprintf("update %s set %s %s",
		t.Name(), strings.Join(set, ","), whereStr)
	var sqlr sql.Result
	var rowAffe int64
	if sqlr, err = t.Db.NamedExec(strSql, param); err != nil {
		err = SqlError{strSql, param, err}
		return
	}
	if rowAffe, err = sqlr.RowsAffected(); err != nil {
		err = SqlError{strSql, param, err}
		return
	}
	if rowAffe == 0 {
		err = SqlError{strSql, param, sql.ErrNoRows}
		return
	}
	return
}

//只有修改过的字段才被更新，where采用全部旧值判断（没有长度的string将不参与，因为oracle会出错）
//如果old、new中有多余字段，则会自动剔除，如果主键缺失，则会出错
func (t *DBTable) Update(oldData, newData map[string]interface{}) (err error) {
	if oldData == nil || len(oldData) == 0 || newData == nil || len(newData) == 0 {
		return fmt.Errorf("data is empty")
	}
	if len(oldData) != len(newData) {
		return fmt.Errorf("the old and new record,field number not same")
	}
	oldData, err = t.checkAndConvertRow(oldData)
	if err != nil {
		return
	}
	newData, err = t.checkAndConvertRow(newData)
	if err != nil {
		return
	}
	where := []string{}
	set := []string{}
	param := map[string]interface{}{}
	icount := 0
	for k, v := range oldData {
		pname := fmt.Sprintf("p%d", icount)
		icount++
		//如果是没有长度的string，即text，以及bytea、datetime则不参与where条件
		//datetime由于有时区和精度的问题，参与的话会比较复杂
		if fld := t.Field(k); fld.GoType() != TypeDatetime && fld.GoType() != TypeBytea && (fld.GoType() != TypeString || fld.MaxLength > 0) {
			if v == nil {
				where = append(where, fmt.Sprintf("%s is null", k))
			} else {
				where = append(where, fmt.Sprintf("%s=:%s_o", k, pname))
				param[pname+"_o"] = v
			}
		}
		if !reflect.DeepEqual(v, newData[k]) {
			set = append(set, fmt.Sprintf("%s=:%s", k, pname))
			param[pname] = newData[k]
		}

	}
	//没有字段被更新，则直接返回
	if len(set) == 0 {
		return
	}
	var sqlr sql.Result
	var rowAffe int64
	strSql := fmt.Sprintf("update %s set %s where %s", t.Name(),
		strings.Join(set, ","), strings.Join(where, " and "))
	if sqlr, err = t.Db.NamedExec(strSql, param); err != nil {
		err = SqlError{strSql, param, err}
		return
	}
	if rowAffe, err = sqlr.RowsAffected(); err != nil {
		err = SqlError{strSql, param, err}
		return
	}
	if rowAffe == 0 {
		err = SqlError{strSql, param, sql.ErrNoRows}
		return
	}
	return
}

//保存一个记录，先尝试用keyvalue去update，如果更新到记录为0再insert，
//逻辑上是正确的，同时，速度也会有保障
func (t *DBTable) Save(row map[string]interface{}) error {

	data, err := t.checkAndConvertRow(row)
	if err != nil {
		return err
	}
	where := []string{}
	set := []string{}
	param := map[string]interface{}{}
	icount := 0
	//用于快速检查主键
	keyIndex := map[string]bool{}
	for _, v := range t.PrimaryKeys() {
		keyIndex[v] = true
	}

	for k, v := range data {
		pname := fmt.Sprintf("p%d", icount)
		param[pname] = v
		icount++
		//非主键才更新
		if _, ok := keyIndex[k]; !ok {
			set = append(set, fmt.Sprintf("%s=:%s", k, pname))
		} else {
			where = append(where, fmt.Sprintf("%s=:%s", k, pname))
		}
	}
	//没有字段被更新，则说明是仅有主键字段，则需要进行exits检查
	if len(set) == 0 {
		if ok, err := t.Exists(data); err != nil {
			return err
		} else if ok {
			return nil
		} else {
			return t.Insert([]map[string]interface{}{data})
		}
	}
	//先更新
	var sqlr sql.Result
	var rowAffe int64
	strSql := fmt.Sprintf("update %s set %s where %s", t.Name(),
		strings.Join(set, ","), strings.Join(where, " and "))
	if sqlr, err = t.Db.NamedExec(strSql, param); err != nil {

		log.WithFields(log.Fields{
			"sql":   strSql,
			"param": param,
			"err":   err.Error(),
		}).Debug("sql error")
		return SqlError{strSql, param, err}
	}
	if rowAffe, err = sqlr.RowsAffected(); err != nil {
		return SqlError{strSql, param, err}
	}
	if rowAffe > 0 {
		return nil
	}
	//再插入
	return t.Insert([]map[string]interface{}{data})
}

//将一批记录替换成另一批记录，自动删除旧在新中不存在，插入新在旧中不存在的，更新主键相同的
func (t *DBTable) Replace(oldRows, newRows []map[string]interface{}) (err error) {
	pkNames := t.PrimaryKeys()
	updateRowsOld, updateRowsNew := mapfun.Intersection(oldRows, newRows, pkNames)

	if err = t.Delete(mapfun.Difference(oldRows, newRows, pkNames)); err != nil {
		return
	}
	for i, v := range updateRowsOld {
		if err = t.Update(v, updateRowsNew[i]); err != nil {
			return
		}
	}
	err = t.Insert(mapfun.Difference(newRows, oldRows, pkNames))
	return
}

func (t *DBTable) FetchColumns() {
	type columnIndex struct {
		Owner      string `db:"INDEXOWNER"`
		IndexName  string `db:"INDEXNAME"`
		ColumnName string `db:"COLUMNNAME"`
	}
	columns := []*DBTableColumn{}
	indexColumns := []*columnIndex{}
	var schema string
	switch t.Db.DriverName() {
	case "postgres":
		if len(t.Schema) > 0 {
			schema = t.Schema
		} else {
			schema = safe.String(MustGetSqlFun(t.Db, "select upper(current_schema())", nil))
		}
		strSql := fmt.Sprintf(`select upper(column_name) as "DBNAME",
					(case when is_nullable='YES' then true else false end) as "DBNULL",
					(case when data_type in ('text', 'character varying')
						then 'STR'
						when  data_type in ('integer','bigint')
						then 'INT'
						when data_type in ('timestamp with time zone', 'timestamp without time zone')
						then 'DATE'
						when data_type in('numeric','double precision','real')
						then 'FLOAT'
						when data_type ='bytea'
						then 'BYTEA'
						else data_type
					end) as "DBTYPE",
					(case when character_maximum_length is null then 0 else character_maximum_length end) as "DBMAXLENGTH",
					(SELECT format_type(a.atttypid, a.atttypmod)
						FROM pg_attribute a 
							JOIN pg_class b ON (a.attrelid = b.relfilenode)
							JOIN pg_namespace c ON (c.oid = b.relnamespace)
						WHERE
							b.relname = outa.table_name AND
							c.nspname = outa.table_schema AND
							a.attname = outa.column_name) as "TRUETYPE"
				from information_schema.columns outa
				where table_schema ilike '%s' and table_name ilike '%s'`, schema, t.TableName)
		if err := t.Db.Select(&columns, strSql); err != nil {
			log.Panic(SqlError{strSql, nil, err})
		}
		strSql = fmt.Sprintf(`select
					(select nspname from pg_namespace where oid=i.relnamespace) as "INDEXOWNER",
					i.relname as "INDEXNAME",
				    upper(min(a.attname)) as "COLUMNNAME"
				from
				    pg_class t,
				    pg_class i,
				    pg_index ix,
				    pg_attribute a,
				    pg_namespace tn
				where
				    t.oid = ix.indrelid
				    and i.oid = ix.indexrelid
				    and a.attrelid = t.oid
				    and t.relnamespace=tn.oid 
				    and upper(tn.nspname) = '%s'
				    and a.attnum = ANY(ix.indkey)
				    and t.relkind = 'r'
				    and upper(t.relname) =$1
				group by
				   t.relname,
				   i.relnamespace,
				   i.relname
				having count(*)=1
				order by
				    t.relname,
				    i.relname;`, schema)
		if err := t.Db.Select(&indexColumns, strSql, t.TableName); err != nil {
			log.Panic(SqlError{strSql, t.TableName, err})
		}
	case "oci8":
		if len(t.Schema) > 0 {
			schema = t.Schema
		} else {
			schema = safe.String(MustGetSqlFun(t.Db, "select user from dual", nil))
		}
		strSql := fmt.Sprintf(`select column_name as "DBNAME",
					decode(nullable,'Y',1,0) as "DBNULL",
					(case when data_type in ('CLOB','VARCHAR', 'VARCHAR2')
						then 'STR'
						when  data_type ='NUMBER' AND DATA_PRECISION IS NULL AND DATA_SCALE = 0 
						then 'INT'
						when data_type ='DATE'
						then 'DATE'
						when data_type in('NUMBER','BINARY_DOUBLE')
						then 'FLOAT'
						when data_type ='BLOB'
						then 'BYTEA'
						else data_type
					end) as "DBTYPE",
					CHAR_LENGTH as "DBMAXLENGTH",
					data_type||
						case
						when data_precision is not null and nvl(data_scale,0)>0 then '('||data_precision||','||data_scale||')'
						when data_precision is not null and nvl(data_scale,0)=0 then '('||data_precision||')'
						when data_precision is null and data_scale is not null then '(*,'||data_scale||')'
						when char_length>0 then '('||char_length|| case char_used 
						                                                         when 'B' then ' Byte'
						                                                         when 'C' then ' Char'
						                                                         else null 
						                                           end||')'
						end as "TRUETYPE"
				from ALL_TAB_COLUMNS 
				where owner='%s' and table_name='%s'
				order by column_id`, schema, t.TableName)
		if err := t.Db.Select(&columns, strSql); err != nil {
			log.Panic(SqlError{strSql, nil, err})
		}
		strSql = fmt.Sprintf(`SELECT min(index_owner) as "INDEXOWNER",
					index_name as "INDEXNAME",min(column_name) as "COLUMNNAME"
				from all_ind_columns a
				where table_owner='%s' and table_name = :name and
					exists(select 1 from all_indexes b where 
						a.index_owner=b.owner and a.index_name =b.index_name and 
						UNIQUENESS ='NONUNIQUE')
				group by index_name having count(*)=1`, schema)
		if err := t.Db.Select(&indexColumns, strSql, t.TableName); err != nil {
			log.Panic(SqlError{strSql, t.TableName, err})
		}
	case "mysql":
		if len(t.Schema) > 0 {
			schema = t.Schema
		} else {
			schema = safe.String(MustGetSqlFun(t.Db, "select upper(SCHEMA())", nil))
		}
		strSql := fmt.Sprintf(`select 
					upper(column_name) as DBNAME,
				    (case when is_nullable='YES' then 1 else 0 end) as DBNULL,
				    (case when data_type in('varchar','text','char') then 'STR'
						  when data_type ='int' then 'INT'
						  when data_type in('decimal','double') then 'FLOAT'
				          when data_type ='blob' then 'BYTEA'
				          when data_type in('date','datetime') then 'DATE'
				    end) as DBTYPE,
				    ifnull(CHARACTER_MAXIMUM_LENGTH,0) as DBMAXLENGTH,
					column_type as TRUETYPE
				from information_schema.columns 
				where upper(table_name)=? and upper(table_schema)= '%s'
				order by ORDINAL_POSITION`, schema)
		if err := t.Db.Select(&columns, strSql, t.TableName); err != nil {
			log.Panic(SqlError{strSql, t.TableName, err})
		}
		strSql = `SELECT INDEX_SCHEMA AS INDEXOWNER,
					INDEX_NAME as INDEXNAME,
					COLUMN_NAME AS COLUMNNAME
				FROM INFORMATION_SCHEMA.STATISTICS 
				WHERE upper(table_schema) = '%s' and upper(table_name)=?
				group by index_name having count(*)=1
				ORDER BY table_name, index_name, seq_in_index`
		if err := t.Db.Select(&indexColumns, strSql, t.TableName); err != nil {
			log.Panic(SqlError{strSql, t.TableName, err})
		}
	case "sqlite3":
		strSql := fmt.Sprintf(`PRAGMA table_info(%s)`, t.TableName)
		result, _, err := QueryRecord(t.Db, strSql, nil)
		if err != nil {
			log.Panic(SqlError{strSql, nil, err})
		}
		for _, row := range result {
			c := &DBTableColumn{
				Name: safe.String(row["NAME"]),
			}
			c.Type, c.MaxLength = sqliteType(safe.String(row["TYPE"]))
			c.TrueType = safe.String(row["TYPE"])
			c.Null = safe.Int(row["NOTNULL"]) != 1
			columns = append(columns, c)
		}
		strSql = fmt.Sprintf("PRAGMA index_list(%s)", t.TableName)
		result, _, err = QueryRecord(t.Db, strSql, nil)
		if err != nil {
			log.Panic(SqlError{strSql, t.TableName, err})
		}
		for _, row := range result {
			indexName := safe.String(row["NAME"])
			//每个索引再去找定义
			strSql = fmt.Sprintf("PRAGMA index_info(%s)", indexName)
			indexColumnList, _, err := QueryRecord(t.Db, strSql, nil)
			if err != nil {
				log.Panic(SqlError{strSql, nil, err})
			}
			//只找出一个字段的索引,并且不是主键索引
			if len(indexColumnList) == 1 && (len(t.PrimaryKeys()) > 1 ||
				safe.String(indexColumnList[0]["NAME"]) != t.PrimaryKeys()[0]) {
				indexColumns = append(indexColumns, &columnIndex{
					"", indexName, safe.String(indexColumnList[0]["NAME"])})
			}
		}
	default:
		log.Panic(fmt.Errorf("not impl FetchColumns"))
	}
	//注意indexColumns中可能含有非表字段的名称，例如oracle中的function index
	indexColumnsMap := map[string]*columnIndex{}
	for _, s := range indexColumns {
		indexColumnsMap[strings.ToUpper(s.ColumnName)] = s
	}
	keyColumnsMap := map[string]bool{}
	for _, s := range t.PrimaryKeys() {
		keyColumnsMap[strings.ToUpper(s)] = true
	}
	for _, v := range columns {
		v.Name = strings.ToUpper(v.Name)
		//对于主键，统一不赋予索引标识
		//if _, ok := keyColumnsMap[v.Name]; ok {
		//	continue
		//}
		//组合主键，有时需要单字段索引

		if s, ok := indexColumnsMap[v.Name]; ok {
			v.Index = true
			v.IndexName = s.IndexName
			if len(t.Schema) > 0 || //如果是其他schema的表，则必定带上schema
				strings.ToUpper(s.Owner) != schema { //如果index不和表在同一个schema中，也带上schema
				v.IndexName = s.Owner + "." + v.IndexName
			}
		}
	}
	//保存获取信息时的数据库驱动名称
	for i, _ := range columns {
		columns[i].FetchDriver = t.Db.DriverName()
	}
	t.columns = columns
	t.refreshColumnsMap()
	t.columnsNames = nil
}
func (t *DBTable) refreshColumnsMap() {
	t.columnsMap = map[string]*DBTableColumn{}
	for _, col := range t.columns {
		t.columnsMap[col.Name] = col
	}
}

//克隆一个table，复制结构定义
func (t *DBTable) Clone() *DBTable {
	result := NewTable(t.Db, t.Name())
	cols := []*DBTableColumn{}
	for _, v := range t.AllField() {
		cols = append(cols, v.Clone())
	}
	result.Define(cols, t.PrimaryKeys())
	return result
}
func (t *DBTable) AllField() []*DBTableColumn {
	if t.columns == nil {
		t.FetchColumns()
	}
	return t.columns
}
func sqliteType(typeName string) (string, int) {
	/*
		<1> 如果声明类型包含”INT”字符串，那么这个列被赋予INTEGER近似
		<2> 如果这个列的声明类型包含”CHAR”，”CLOB”，或者”TEXT”中的任意一个，那么这个列就有了TEXT近似。注意类型VARCHAR包含了”CHAR”字符串，那么也就被赋予了TEXT近似
		<3> 如果列的声明类型中包含了字符串”BLOB”或者没有为其声明类型，这个列被赋予NONE近似
		<4> 其他的情况，列被赋予NUMERIC近似
	*/
	typeName = strings.ToUpper(typeName)
	if strings.Contains(typeName, "INT") {
		return "INT", 0
	}
	if strings.Contains(typeName, "CHAR") ||
		strings.Contains(typeName, "CLOB") ||
		strings.Contains(typeName, "TEXT") {
		length := "-1"
		if ts := strings.Split(typeName, "("); len(ts) > 1 {
			length = ts[1]
			length = length[:len(length)-1]
		}
		i, _ := strconv.ParseInt(length, 10, 64)
		return "STR", int(i)
	}
	if strings.Contains(typeName, "BLOB") || strings.Contains(typeName, "BYTEA") ||
		len(typeName) == 0 {
		return "BYTEA", 0
	}
	if strings.Contains(typeName, "DATE") || strings.Contains(typeName, "TIME") {
		return "DATE", 0
	}
	return "FLOAT", 0
}
func (t *DBTable) Field(name string) *DBTableColumn {
	if len(t.columnsMap) == 0 {
		t.FetchColumns()
	}
	if col, ok := t.columnsMap[name]; ok {
		return col
	} else {
		return nil
	}
}

//采用脚本的方式定义表，如下：
//  a str(3) not null
//  b int
//  c date not null index
//  primary key(a,c)
func (t *DBTable) DefineScript(src string) {
	lineReg, err := regexp.Compile(`(?i)([\p{Han}_a-zA-Z0-9]+)(\s+bytea|\s+date|\s+float|\s+int|\s+str\([0-9]+\)|\s+str|)(\s+null|\s+not null|)(\s+index|)`)
	if err != nil {
		log.Panic(err)
	}
	pks := []string{}
	columns := []*DBTableColumn{}
	var prevColumn *DBTableColumn
	for i, line := range strings.Split(strings.Replace(src, "\r\n", "\n", -1), "\n") {
		//这里全部转换成小写，后面的字段变更判断就需要增加大小写忽略的逻辑
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		//如果是主键定义
		if strings.HasPrefix(line, "primary key(") {
			for _, v := range strings.Split(line[12:len(line)-1], ",") {
				pks = append(pks, strings.TrimSpace(v))
			}
		} else {
			lineList := lineReg.FindStringSubmatch(line)
			if len(lineList) == 0 {
				log.Panic(fmt.Errorf("line %d:%s error", i, line))
			}
			//第一个是整行，需要去除
			lineList = lineList[1:]
			if len(lineList) == 0 {
				log.Panic(fmt.Errorf("line %d:%s error", i, line))
			}
			colName := lineList[0]
			if len(strings.TrimSpace(lineList[1])) == 0 {
				//如果只有列名，则自动从上一个字段取出数据类型等定义
				if prevColumn == nil {
					log.Panic(fmt.Errorf("line %d:%s not data type", i, line))
				} else {
					col := prevColumn.Clone()
					col.Name = colName
					columns = append(columns, col)
					prevColumn = col
					continue
				}
			}
			dataType := strings.ToLower(strings.TrimSpace(lineList[1]))
			notNull := false
			index := false
			var maxLength int64 = -1
			if len(lineList) > 2 {
				switch str := strings.ToLower(strings.TrimSpace(lineList[2])); str {
				case "not null":
					notNull = true
				case "null":
					notNull = false
				case "":
				default:
					log.Panic(fmt.Errorf("line %d:%s ,error define %s", i, line, str))
				}
			}
			if len(lineList) > 3 {
				switch str := strings.ToLower(strings.TrimSpace(lineList[3])); str {
				case "index":
					index = true
				case "":
				default:
					log.Panic(fmt.Errorf("line %d:%s ,error define %s", i, line, str))
				}
			}
			if strings.HasPrefix(dataType, "str(") {
				maxLength, err = strconv.ParseInt(dataType[4:len(dataType)-1], 10, 64)
				if err != nil {
					log.Panic(err)
				}
				dataType = "STR"
			} else {
				dataType = strings.ToUpper(dataType)
			}
			prevColumn = &DBTableColumn{
				Name:      colName,
				Type:      dataType,
				MaxLength: int(maxLength),
				Null:      !notNull,
				Index:     index,
			}
			columns = append(columns, prevColumn)
		}
	}
	t.Define(columns, pks)
}

//手工赋值
func (t *DBTable) Define(columns []*DBTableColumn, pk []string) {
	//所有是主键的字段如果没有长度，则设置为300
	for _, col := range columns {
		for _, k := range pk {
			if col.Name == k {
				if col.Type == "STR" && col.MaxLength <= 0 {
					col.MaxLength = 300
				}
				break
			}
		}
	}
	t.columns = columns
	t.refreshColumnsMap()
	//检查主键是否合法
	for _, col := range pk {
		if t.Field(col) == nil {
			log.WithFields(log.Fields{
				"table": t.TableName,
				"col":   col,
			}).Panic("primary key column not exists")
		}
	}
	t.primaryKeys = pk
}
func (t *DBTable) Create() error {
	sch := &TableSchema{
		NewTable: t,
	}
	return sch.Update()
}

//Merge 将另一个表中的数据合并进本表，要求两个表的主键相同,相同主键的被覆盖
//skipColumns指定跳过update的字段清单
func (t *DBTable) Merge(tabName string, skipUpdateColumns ...string) error {
	join := []string{}
	updateSet := []string{}
	insertColumns := []string{}
	insertValues := []string{}
	pkMap := map[string]bool{}
	for _, v := range t.PrimaryKeys() {
		pkMap[v] = true
		join = append(join, fmt.Sprintf("dest.%s = src.%s", v, v))
	}
	for _, field := range t.AllField() {
		//非主键的才更新
		if _, ok := pkMap[field.Name]; !ok {
			bfound := false

			for _, one := range skipUpdateColumns {
				if one == field.Name {
					bfound = true
					break
				}
			}
			//只有不是跳过的，才update
			if !bfound {
				updateSet = append(updateSet, fmt.Sprintf("dest.%s = src.%s", field.Name, field.Name))
			}
		}
		insertColumns = append(insertColumns, fmt.Sprintf("dest.%s", field.Name))
		insertValues = append(insertValues, fmt.Sprintf("src.%s", field.Name))
	}
	switch t.Db.DriverName() {
	case "oci8":
		strSql := fmt.Sprintf(`
MERGE INTO %s dest
USING(select * from %s) src 
ON(%s)
WHEN MATCHED THEN UPDATE SET
	%s
WHEN NOT MATCHED THEN INSERT
	(%s)
	values
	(%s)`, t.Name(), tabName,
			strings.Join(join, " and "),
			strings.Join(updateSet, ",\n"),
			strings.Join(insertColumns, ","),
			strings.Join(insertValues, ","))
		if _, err := t.Db.Exec(strSql); err != nil {
			return SqlError{strSql, nil, err}
		}
	default:
		log.Panic("not impl Merge")
	}
	return nil
}

//更新一个表的结构至数据库中，会自动处理表改名、字段改名以及字段修改、索引修改等操作
func (t *DBTable) UpdateSchema() error {
	sch := &TableSchema{
		NewTable: t,
	}
	if len(t.FormerName) > 0 {
		//如果有曾用名，则需验证曾用名不能重复
		uname := map[string]bool{
			t.Name(): true,
		}
		for _, v := range t.FormerName {
			if _, ok := uname[v]; ok {
				return fmt.Errorf("FormerName:%s dup", v)
			}
		}
		//并根据曾用名去获取之前的表结构
		for _, v := range t.FormerName {
			if b, _ := TableExists(t.Db, v); b {
				sch.OldTable = NewTable(t.Db, v)
				sch.OldTable.FetchColumns()
				break
			}
		}
	}
	//如果曾用名的表找不到，则说明数据库结构都已经更新到最新，旧表就用本来的名称
	if sch.OldTable == nil {
		b, err := TableExists(t.Db, t.Name())
		if err != nil {
			return err
		}
		if b {
			sch.OldTable = NewTable(t.Db, t.Name())
			sch.OldTable.FetchColumns()
		}
	}
	return sch.Update()
}
