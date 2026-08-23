package typescript

const typeScriptSQLConnectionRuntime = `
function __trbOpenSQL(adapter: string, source: string): SQL {
  if (adapter === "sqlite") return new SQL({ adapter: "sqlite", filename: source });
  if (adapter !== "mysql" || !source.startsWith("mysql://")) return new SQL(source);
  const parsed = new URL(source);
  const setting = parsed.searchParams.get("allowPublicKeyRetrieval");
  if (setting === null) return new SQL(source);
  if (setting !== "true" && setting !== "false") {
    throw new Error("MySQL allowPublicKeyRetrieval must be true or false");
  }
  parsed.searchParams.delete("allowPublicKeyRetrieval");
  return new SQL({
    adapter: "mysql",
    url: parsed.toString(),
    allowPublicKeyRetrieval: setting === "true",
  });
}
`
