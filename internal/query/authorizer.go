package query

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// attachedDB is the schema name used when ATTACH-ing a business database.
const attachedDB = "src"

// authorizer policies are keyed by the raw sqlite connection pointer.
var authorizers = struct {
	mu sync.Mutex
	m  map[uintptr]authPolicy
}{m: map[uintptr]authPolicy{}}

type authPolicy struct {
	views map[string]struct{}
}

// installAuthorizer rejects writes and direct reads of the attached business
// file. Reads that SQLite performs while expanding one of our temp views are
// allowed, so row filters stay inside the view definition.
func installAuthorizer(conn *sql.Conn, views []string) error {
	allowed := make(map[string]struct{}, len(views))
	for _, name := range views {
		allowed[strings.ToLower(name)] = struct{}{}
	}
	return conn.Raw(func(driverConn any) error {
		tls, db, err := sqliteHandles(driverConn)
		if err != nil {
			return err
		}
		authorizers.mu.Lock()
		authorizers.m[db] = authPolicy{views: allowed}
		authorizers.mu.Unlock()
		rc := sqlite3.Xsqlite3_set_authorizer(tls, db, cFuncPointer(authorizerTrampoline), db)
		if rc != sqlite3.SQLITE_OK {
			return fmt.Errorf("设置 SQL 授权回调失败 (%d)", rc)
		}
		return nil
	})
}

func clearAuthorizer(conn *sql.Conn) {
	_ = conn.Raw(func(driverConn any) error {
		_, db, err := sqliteHandles(driverConn)
		if err != nil {
			return nil
		}
		authorizers.mu.Lock()
		delete(authorizers.m, db)
		authorizers.mu.Unlock()
		return nil
	})
}

// sqliteHandles reads the unexported modernc connection header. The pinned
// driver keeps db and tls as the first two fields of its connection struct.
func sqliteHandles(driverConn any) (*libc.TLS, uintptr, error) {
	v := reflect.ValueOf(driverConn)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, 0, fmt.Errorf("无效的 sqlite 连接")
	}
	base := unsafe.Pointer(v.Pointer())
	db := *(*uintptr)(base)
	tls := *(**libc.TLS)(unsafe.Add(base, unsafe.Sizeof(uintptr(0))))
	if db == 0 || tls == nil {
		return nil, 0, fmt.Errorf("无法读取 sqlite 连接")
	}
	return tls, db, nil
}

func cFuncPointer[T any](f T) uintptr {
	return *(*uintptr)(unsafe.Pointer(&struct{ f T }{f}))
}

func authorizerTrampoline(tls *libc.TLS, pArg uintptr, action int32, a, b, c, d uintptr) int32 {
	authorizers.mu.Lock()
	policy, ok := authorizers.m[pArg]
	authorizers.mu.Unlock()
	if !ok {
		return sqlite3.SQLITE_DENY
	}
	arg1 := libc.GoString(a)
	arg2 := libc.GoString(b)
	arg3 := libc.GoString(c)
	arg4 := libc.GoString(d)
	_ = tls
	if allowAuth(policy, action, arg1, arg2, arg3, arg4) {
		return sqlite3.SQLITE_OK
	}
	return sqlite3.SQLITE_DENY
}

func allowAuth(policy authPolicy, action int32, arg1, arg2, arg3, arg4 string) bool {
	switch action {
	case sqlite3.SQLITE_SELECT, sqlite3.SQLITE_TRANSACTION, sqlite3.SQLITE_SAVEPOINT, sqlite3.SQLITE_RECURSIVE:
		return true
	case sqlite3.SQLITE_FUNCTION:
		switch strings.ToLower(arg2) {
		case "load_extension", "writefile", "readfile", "fts3_tokenizer", "edit":
			return false
		default:
			return true
		}
	case sqlite3.SQLITE_READ:
		return allowRead(policy, arg1, arg3, arg4)
	default:
		return false
	}
}

func allowRead(policy authPolicy, table, database, via string) bool {
	dbName := strings.ToLower(database)
	tableName := strings.ToLower(table)
	if dbName == attachedDB {
		// Direct reads of the business file are denied. View expansion sets
		// the innermost view name, and only our temp views may do that.
		if via == "" {
			return false
		}
		_, ok := policy.views[strings.ToLower(via)]
		return ok
	}
	if strings.HasPrefix(tableName, "sqlite_") {
		return dbName == "main" || dbName == "temp" || dbName == ""
	}
	if _, ok := policy.views[tableName]; ok {
		return true
	}
	// Columns of a temp view are reported with that view name.
	return false
}
