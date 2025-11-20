package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) exportJSON(c *gin.Context, table string) error {
	_, rows, err := s.fetchAllRows(table)
	if err != nil {
		return err
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, table))
	c.Header("Content-Type", "application/json")
	return json.NewEncoder(c.Writer).Encode(rows)
}

func (s *Server) exportCSV(c *gin.Context, table string) error {
	columns, rows, err := s.fetchAllRows(table)
	if err != nil {
		return err
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, table))
	c.Header("Content-Type", "text/csv")

	writer := csv.NewWriter(c.Writer)
	if err := writer.Write(columns); err != nil {
		return err
	}
	for _, row := range rows {
		record := make([]string, len(columns))
		for i, col := range columns {
			record[i] = csvValue(row[col])
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func (s *Server) exportSQL(c *gin.Context, table string) error {
	schema, err := s.getTableSchema(table)
	if err != nil {
		return err
	}
	columns, rows, err := s.fetchAllRows(table)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(schema + ";\n")
	buf.WriteString(fmt.Sprintf("DELETE FROM %s;\n", QuoteIdentifier(table)))

	quotedCols := make([]string, len(columns))
	for i, col := range columns {
		quotedCols[i] = QuoteIdentifier(col)
	}
	colList := strings.Join(quotedCols, ", ")
	for _, row := range rows {
		values := make([]string, len(columns))
		for i, col := range columns {
			values[i] = formatSQLValue(row[col])
		}
		buf.WriteString(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);\n", QuoteIdentifier(table), colList, strings.Join(values, ", ")))
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.sql"`, table))
	c.Header("Content-Type", "application/sql")
	_, err = c.Writer.Write(buf.Bytes())
	return err
}

func csvValue(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatSQLValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(val, "'", "''") + "'"
	case []byte:
		return "'" + strings.ReplaceAll(string(val), "'", "''") + "'"
	case bool:
		if val {
			return "1"
		}
		return "0"
	case int:
		return fmt.Sprintf("%d", val)
	case int8:
		return fmt.Sprintf("%d", val)
	case int16:
		return fmt.Sprintf("%d", val)
	case int32:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case uint:
		return fmt.Sprintf("%d", val)
	case uint8:
		return fmt.Sprintf("%d", val)
	case uint16:
		return fmt.Sprintf("%d", val)
	case uint32:
		return fmt.Sprintf("%d", val)
	case uint64:
		return fmt.Sprintf("%d", val)
	case float32:
		return fmt.Sprintf("%g", val)
	case float64:
		return fmt.Sprintf("%g", val)
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", val), "'", "''") + "'"
	}
}
