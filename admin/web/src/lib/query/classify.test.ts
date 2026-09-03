import { describe, expect, it } from "vitest";
import { classifyStatement } from "./classify";

describe("classifyStatement", () => {
	it("reads a select, with, show, explain, table or values", () => {
		for (const sql of [
			"SELECT * FROM users",
			"  select 1",
			"WITH t AS (SELECT 1) SELECT * FROM t",
			"SHOW server_version",
			"EXPLAIN SELECT 1",
			"TABLE users",
			"VALUES (1), (2)",
		]) {
			expect(classifyStatement(sql)).toBe("read");
		}
	});

	it("writes an insert, update, delete or DDL", () => {
		for (const sql of [
			"INSERT INTO users (name) VALUES ('a')",
			"update users set active = true",
			"DELETE FROM users WHERE id = 1",
			"CREATE TABLE t (id int)",
			"ALTER TABLE t ADD COLUMN c text",
			"DROP TABLE t",
			"TRUNCATE users",
			"grant select on t to r",
		]) {
			expect(classifyStatement(sql)).toBe("write");
		}
	});

	it("writes a WITH that wraps a data-modifying CTE", () => {
		expect(classifyStatement("WITH d AS (DELETE FROM users RETURNING *) SELECT * FROM d")).toBe(
			"write",
		);
		expect(
			classifyStatement("with moved as (insert into archive select * from users returning id) select * from moved"),
		).toBe("write");
	});

	it("does not treat a DML word inside a literal, comment or quoted name as a write", () => {
		expect(classifyStatement("WITH x AS (SELECT 'delete' AS action) SELECT * FROM x")).toBe("read");
		expect(classifyStatement("WITH x AS (SELECT 1 /* delete */) SELECT * FROM x")).toBe("read");
		expect(classifyStatement(`WITH x AS (SELECT "delete" FROM t) SELECT * FROM x`)).toBe("read");
		expect(classifyStatement("WITH x AS (SELECT $$update$$ AS s) SELECT * FROM x")).toBe("read");
	});

	it("classifies by the first keyword, past leading comments and whitespace", () => {
		expect(classifyStatement("-- a note\nSELECT 1")).toBe("read");
		expect(classifyStatement("/* block */ UPDATE users SET x = 1")).toBe("write");
		expect(classifyStatement("\n\t  select 1")).toBe("read");
	});

	it("reads an empty or unrecognised statement, leaving the server to judge it", () => {
		expect(classifyStatement("")).toBe("read");
		expect(classifyStatement("   ")).toBe("read");
	});
});
