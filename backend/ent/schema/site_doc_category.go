package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type SiteDocCategory struct{ ent.Schema }

func (SiteDocCategory) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "site_doc_categories", Checks: map[string]string{"site_doc_categories_version_check": "version > 0", "site_doc_categories_position_check": "position >= 0"}}}
}
func (SiteDocCategory) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(80), field.Int("position").Default(0), field.Int("version").Default(1),
	}
}
