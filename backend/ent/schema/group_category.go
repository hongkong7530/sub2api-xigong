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

// GroupCategory is an admin-managed catalog for organizing routing groups in the user picker.
type GroupCategory struct {
	ent.Schema
}

func (GroupCategory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "group_categories"},
	}
}

func (GroupCategory) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (GroupCategory) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(40).
			NotEmpty(),
		field.Int("sort_order").
			Default(0),
	}
}

func (GroupCategory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("groups", Group.Type),
	}
}

func (GroupCategory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sort_order"),
	}
}
