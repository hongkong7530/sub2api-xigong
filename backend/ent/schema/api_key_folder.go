package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// APIKeyFolder is a per-user folder for organizing the owner's API keys.
type APIKeyFolder struct {
	ent.Schema
}

func (APIKeyFolder) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_key_folders"},
	}
}

func (APIKeyFolder) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (APIKeyFolder) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("name").
			MaxLen(40).
			NotEmpty(),
		field.Int("sort_order").
			Default(0),
	}
}

func (APIKeyFolder) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("api_key_folders").
			Field("user_id").
			Unique().
			Required(),
		edge.To("api_keys", APIKey.Type),
	}
}

func (APIKeyFolder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("user_id", "sort_order"),
	}
}
