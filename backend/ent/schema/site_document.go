package schema

import (
	"encoding/json"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"
)

type SiteDocument struct{ ent.Schema }

func (SiteDocument) Annotations() []schema.Annotation {
	// 排序取值范围约束依赖 JSON 路径与类型转换语法，仅由迁移在 PostgreSQL 上建立，
	// Ent 建表保持各测试方言（含 SQLite）可用。
	return []schema.Annotation{entsql.Annotation{Table: "site_documents", Checks: map[string]string{"site_documents_version_check": "version > 0"}}}
}
func (SiteDocument) Fields() []ent.Field {
	return []ent.Field{
		field.String("slug").MaxLen(96).Unique(),
		field.JSON("draft", json.RawMessage{}),
		field.JSON("published", json.RawMessage{}).Optional(),
		field.Int("version").Default(1),
		field.Time("updated_at").Default(time.Now).Annotations(entsql.DefaultExprs(map[string]string{dialect.SQLite: "CURRENT_TIMESTAMP", dialect.Postgres: "now()"})),
		field.Time("published_at").Optional().Nillable(),
	}
}

func (SiteDocument) Indexes() []ent.Index {
	return []ent.Index{index.Fields("id").StorageKey("site_documents_published_idx").Annotations(entsql.IndexWhere("published IS NOT NULL"))}
}
