package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type SitePartner struct{ ent.Schema }

func (SitePartner) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "site_partners", Checks: map[string]string{"site_partners_version_check": "version > 0", "site_partners_position_check": "position >= 0"}}}
}
func (SitePartner) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100), field.Text("description").Default(""), field.Text("url"), field.Text("icon").Default(""), field.Text("thumbnail").Default(""),
		field.String("tag").MaxLen(120).Default(""), field.Int("position").Default(0), field.Bool("enabled").Default(true), field.Int("version").Default(1),
	}
}
