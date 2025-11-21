package server

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

type Server struct {
	db     *sql.DB
	router *gin.Engine
	static http.FileSystem
}

func New(dbPath string, static http.FileSystem) (*Server, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite file: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite file: %w", err)
	}

	s := &Server{
		db:     db,
		router: gin.Default(),
		static: static,
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) Run(addr string) error {
	return s.router.Run(addr)
}

func (s *Server) registerRoutes() {
	api := s.router.Group("/api")
	{
		api.GET("/tables", s.handleListTables)
		api.GET("/tables/:table", s.handleGetTableData)
		api.GET("/tables/:table/schema", s.handleGetTableSchema)
		api.POST("/tables/:table/rows", s.handleInsertRow)
		api.PATCH("/tables/:table/rows/:rowid", s.handleUpdateRow)
		api.DELETE("/tables/:table/rows/:rowid", s.handleDeleteRow)
		api.GET("/tables/:table/export", s.handleExportTable)
		api.POST("/query", s.handleExecuteQuery)
		api.GET("/indexes", s.handleListIndexes)
		api.GET("/views", s.handleListViews)
	}

	s.router.NoRoute(s.handleSPA)
}

func (s *Server) handleListTables(c *gin.Context) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		tables = append(tables, name)
	}

	c.JSON(http.StatusOK, gin.H{"tables": tables})
}

func (s *Server) handleGetTableData(c *gin.Context) {
	table := c.Param("table")
	if !IsSafeIdentifier(table) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	search := c.DefaultQuery("search", "")
	orderBy := c.DefaultQuery("orderBy", "")
	orderDir := c.DefaultQuery("orderDir", "ASC")

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if orderDir != "ASC" && orderDir != "DESC" {
		orderDir = "ASC"
	}

	// Build WHERE clause for search
	whereClause := ""
	args := []interface{}{}
	if search != "" {
		// Get all columns to search in
		colRows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", QuoteIdentifier(table)))
		if err == nil {
			defer colRows.Close()
			var searchConditions []string
			for colRows.Next() {
				var cid int
				var colName, colType string
				var notnull, pk int
				var dflt sql.NullString
				if err := colRows.Scan(&cid, &colName, &colType, &notnull, &dflt, &pk); err == nil {
					searchConditions = append(searchConditions, fmt.Sprintf("%s LIKE ?", QuoteIdentifier(colName)))
					args = append(args, "%"+search+"%")
				}
			}
			if len(searchConditions) > 0 {
				whereClause = "WHERE (" + strings.Join(searchConditions, " OR ") + ")"
			}
		}
	}

	// Build ORDER BY clause
	orderClause := ""
	if orderBy != "" && IsSafeIdentifier(orderBy) {
		orderClause = fmt.Sprintf("ORDER BY %s %s", QuoteIdentifier(orderBy), orderDir)
	}

	// Build query
	baseQuery := fmt.Sprintf("SELECT rowid as _rowid, * FROM %s", QuoteIdentifier(table))
	query := baseQuery
	if whereClause != "" {
		query += " " + whereClause
	}
	if orderClause != "" {
		query += " " + orderClause
	}
	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var data []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		row := map[string]interface{}{}
		for i, col := range columns {
			row[col] = normalizeValue(values[i])
		}
		data = append(data, row)
	}

	// Get total count
	totalQuery := fmt.Sprintf("SELECT COUNT(1) FROM %s", QuoteIdentifier(table))
	if whereClause != "" {
		totalQuery += " " + whereClause
		totalArgs := args[:len(args)-2] // Remove limit and offset
		var total int
		if err := s.db.QueryRow(totalQuery, totalArgs...).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"columns": columns,
			"rows":    data,
			"total":   total,
			"limit":   limit,
			"offset":  offset,
		})
	} else {
		var total int
		if err := s.db.QueryRow(totalQuery).Scan(&total); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"columns": columns,
			"rows":    data,
			"total":   total,
			"limit":   limit,
			"offset":  offset,
		})
	}
}

func (s *Server) handleUpdateRow(c *gin.Context) {
	table := c.Param("table")
	if !IsSafeIdentifier(table) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}
	rowidStr := c.Param("rowid")
	rowid, err := strconv.ParseInt(rowidStr, 10, 64)
	if err != nil || rowid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rowid"})
		return
	}

	var payload map[string]interface{}
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}
	delete(payload, "_rowid")
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no columns to update"})
		return
	}

	setClauses := make([]string, 0, len(payload))
	values := make([]interface{}, 0, len(payload)+1)
	for col, val := range payload {
		if !IsSafeIdentifier(col) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid column: %s", col)})
			return
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", QuoteIdentifier(col)))
		values = append(values, val)
	}
	values = append(values, rowid)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE rowid = ?", QuoteIdentifier(table), strings.Join(setClauses, ", "))
	res, err := s.db.Exec(query, values...)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "row not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "updated": affected})
}

func (s *Server) handleInsertRow(c *gin.Context) {
	table := c.Param("table")
	if !IsSafeIdentifier(table) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}

	var payload map[string]interface{}
	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}
	if len(payload) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no columns to insert"})
		return
	}

	columns := make([]string, 0, len(payload))
	placeholders := make([]string, 0, len(payload))
	values := make([]interface{}, 0, len(payload))
	for col, val := range payload {
		if !IsSafeIdentifier(col) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid column: %s", col)})
			return
		}
		columns = append(columns, QuoteIdentifier(col))
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		QuoteIdentifier(table),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)
	res, err := s.db.Exec(query, values...)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rowid, _ := res.LastInsertId()
	c.JSON(http.StatusOK, gin.H{"status": "ok", "rowid": rowid})
}

func (s *Server) handleDeleteRow(c *gin.Context) {
	table := c.Param("table")
	if !IsSafeIdentifier(table) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}
	rowidStr := c.Param("rowid")
	rowid, err := strconv.ParseInt(rowidStr, 10, 64)
	if err != nil || rowid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rowid"})
		return
	}

	res, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", QuoteIdentifier(table)), rowid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "row not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "deleted": affected})
}

func (s *Server) handleExportTable(c *gin.Context) {
	table := c.Param("table")
	if !IsSafeIdentifier(table) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}
	format := c.DefaultQuery("format", "csv")

	switch format {
	case "csv":
		if err := s.exportCSV(c, table); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
	case "json":
		if err := s.exportJSON(c, table); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
	case "sql":
		if err := s.exportSQL(c, table); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported format"})
	}
}

func (s *Server) handleSPA(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if s.static == nil || gin.Mode() == gin.TestMode {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	path := strings.TrimPrefix(c.Request.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if !s.serveStatic(c, path) {
		s.serveStatic(c, "index.html")
	}
}

func (s *Server) serveStatic(c *gin.Context, path string) bool {
	file, err := s.static.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	var reader io.ReadSeeker
	if rs, ok := file.(io.ReadSeeker); ok {
		reader = rs
	} else {
		data, err := io.ReadAll(file)
		if err != nil {
			return false
		}
		reader = bytes.NewReader(data)
	}
	http.ServeContent(c.Writer, c.Request, path, info.ModTime(), reader)
	return true
}

func normalizeValue(val interface{}) interface{} {
	switch v := val.(type) {
	case []byte:
		return string(v)
	case nil:
		return nil
	default:
		return v
	}
}

func QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func IsSafeIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r == '_' || r == '.' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}

func (s *Server) fetchAllRows(table string) ([]string, []map[string]interface{}, error) {
	if !IsSafeIdentifier(table) {
		return nil, nil, fmt.Errorf("invalid table name")
	}
	query := fmt.Sprintf("SELECT * FROM %s", QuoteIdentifier(table))
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var data []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := map[string]interface{}{}
		for i, col := range columns {
			row[col] = normalizeValue(values[i])
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return columns, data, nil
}

func (s *Server) getTableSchema(table string) (string, error) {
	var schema sql.NullString
	err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&schema)
	if err != nil {
		return "", err
	}
	if !schema.Valid {
		return "", errors.New("schema not found")
	}
	return schema.String, nil
}

func (s *Server) handleGetTableSchema(c *gin.Context) {
	table := c.Param("table")
	if !IsSafeIdentifier(table) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid table name"})
		return
	}

	// Get table schema SQL
	schema, err := s.getTableSchema(table)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Get column info using PRAGMA
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", QuoteIdentifier(table)))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type ColumnInfo struct {
		CID        int    `json:"cid"`
		Name       string `json:"name"`
		Type       string `json:"type"`
		NotNull    int    `json:"notnull"`
		Default    string `json:"dflt_value"`
		PrimaryKey int    `json:"pk"`
	}

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var dflt sql.NullString
		if err := rows.Scan(&col.CID, &col.Name, &col.Type, &col.NotNull, &dflt, &col.PrimaryKey); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if dflt.Valid {
			col.Default = dflt.String
		}
		columns = append(columns, col)
	}

	// Get indexes
	indexRows, err := s.db.Query(`SELECT name, sql FROM sqlite_master WHERE type='index' AND tbl_name=?`, table)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer indexRows.Close()

	type IndexInfo struct {
		Name string `json:"name"`
		SQL  string `json:"sql"`
	}
	var indexes []IndexInfo
	for indexRows.Next() {
		var idx IndexInfo
		var sqlStr sql.NullString
		if err := indexRows.Scan(&idx.Name, &sqlStr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if sqlStr.Valid {
			idx.SQL = sqlStr.String
		}
		indexes = append(indexes, idx)
	}

	c.JSON(http.StatusOK, gin.H{
		"schema":  schema,
		"columns": columns,
		"indexes": indexes,
	})
}

func (s *Server) handleExecuteQuery(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON payload"})
		return
	}

	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query cannot be empty"})
		return
	}

	// Check if it's a SELECT query (read-only)
	queryUpper := strings.ToUpper(strings.TrimSpace(req.Query))
	isSelect := strings.HasPrefix(queryUpper, "SELECT") || strings.HasPrefix(queryUpper, "WITH")

	if isSelect {
		rows, err := s.db.Query(req.Query)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var data []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			row := map[string]interface{}{}
			for i, col := range columns {
				row[col] = normalizeValue(values[i])
			}
			data = append(data, row)
		}

		c.JSON(http.StatusOK, gin.H{
			"columns": columns,
			"rows":    data,
			"type":    "select",
		})
	} else {
		// Execute write operations (INSERT, UPDATE, DELETE, etc.)
		result, err := s.db.Exec(req.Query)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		affected, _ := result.RowsAffected()
		lastInsertID, _ := result.LastInsertId()

		c.JSON(http.StatusOK, gin.H{
			"type":         "write",
			"rowsAffected": affected,
			"lastInsertId": lastInsertID,
		})
	}
}

func (s *Server) handleListIndexes(c *gin.Context) {
	rows, err := s.db.Query(`SELECT name, tbl_name, sql FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY tbl_name, name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type Index struct {
		Name  string `json:"name"`
		Table string `json:"table"`
		SQL   string `json:"sql"`
	}

	var indexes []Index
	for rows.Next() {
		var idx Index
		var sqlStr sql.NullString
		if err := rows.Scan(&idx.Name, &idx.Table, &sqlStr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if sqlStr.Valid {
			idx.SQL = sqlStr.String
		}
		indexes = append(indexes, idx)
	}

	c.JSON(http.StatusOK, gin.H{"indexes": indexes})
}

func (s *Server) handleListViews(c *gin.Context) {
	rows, err := s.db.Query(`SELECT name, sql FROM sqlite_master WHERE type='view' ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type View struct {
		Name string `json:"name"`
		SQL  string `json:"sql"`
	}

	var views []View
	for rows.Next() {
		var v View
		var sqlStr sql.NullString
		if err := rows.Scan(&v.Name, &sqlStr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if sqlStr.Valid {
			v.SQL = sqlStr.String
		}
		views = append(views, v)
	}

	c.JSON(http.StatusOK, gin.H{"views": views})
}
