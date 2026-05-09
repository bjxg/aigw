package db

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DateTruncExpr returns a platform-appropriate clause.Expr that truncates
// a timestamp column to the given granularity ("day" or "hour").
// This abstracts the difference between:
//   - SQLite: date(col, 'localtime') / strftime('%Y-%m-%d %H:00', col, 'localtime')
//   - PostgreSQL: date_trunc('day', col) / date_trunc('hour', col)
func DateTruncExpr(d *gorm.DB, col string, granularity string) clause.Expr {
	if IsSQLiteDB(d) {
		switch granularity {
		case "day":
			return clause.Expr{SQL: "date(?, 'localtime')", Vars: []interface{}{clause.Column{Table: "", Name: col}}}
		case "hour":
			return clause.Expr{SQL: "strftime('%Y-%m-%d %H:00', ?, 'localtime')", Vars: []interface{}{clause.Column{Table: "", Name: col}}}
		}
	}
	// PostgreSQL and others: use date_trunc
	switch granularity {
	case "day":
		return clause.Expr{SQL: "date_trunc('day', ?)", Vars: []interface{}{clause.Column{Table: "", Name: col}}}
	case "hour":
		return clause.Expr{SQL: "date_trunc('hour', ?)", Vars: []interface{}{clause.Column{Table: "", Name: col}}}
	}
	return clause.Expr{SQL: "?", Vars: []interface{}{clause.Column{Table: "", Name: col}}}
}

// CurrentTimestampExpr returns a clause.Expr that evaluates to the current
// timestamp in a database-agnostic way.
func CurrentTimestampExpr(d *gorm.DB) clause.Expr {
	return clause.Expr{SQL: "CURRENT_TIMESTAMP"}
}
