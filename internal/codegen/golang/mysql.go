package golang

func (g *generator) normalizeMySQLSource(source, errorVariable string) {
	g.requireImport("errors", "")
	g.requireImport("github.com/go-sql-driver/mysql", "trbmysql")
	g.requireImport("net/url", "url")
	g.requireImport("strings", "")

	g.line("if strings.HasPrefix(" + source + ", \"mysql://\") { parsed, err := url.Parse(" + source + "); if err != nil { " + errorVariable + " = err; return }; credentials := parsed.User.Username(); if password, exists := parsed.User.Password(); exists { credentials += \":\" + password }; " + source + " = credentials + \"@tcp(\" + parsed.Host + \")\" + parsed.Path; if parsed.RawQuery != \"\" { " + source + " += \"?\" + parsed.RawQuery } }")
	g.line("trbMySQLConfig, trbMySQLConfigError := trbmysql.ParseDSN(" + source + ")")
	g.line("if trbMySQLConfigError != nil { " + errorVariable + " = trbMySQLConfigError; return }")
	g.line("if setting, exists := trbMySQLConfig.Params[\"allowPublicKeyRetrieval\"]; exists { if setting != \"true\" && setting != \"false\" { " + errorVariable + " = errors.New(\"MySQL allowPublicKeyRetrieval must be true or false\"); return }; delete(trbMySQLConfig.Params, \"allowPublicKeyRetrieval\"); " + source + " = trbMySQLConfig.FormatDSN() }")
}
