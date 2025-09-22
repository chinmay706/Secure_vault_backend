package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"
)

func (ec *executionContext) unmarshalInputDateTime(ctx context.Context, obj interface{}) (time.Time, error) {
str, ok := obj.(string)
if !ok {
return time.Time{}, fmt.Errorf("DateTime must be a string")
}
return time.Parse(time.RFC3339, str)
}

func (ec *executionContext) _DateTime(ctx context.Context, sel interface{}, obj *time.Time) graphql.Marshaler {
if obj == nil {
return graphql.Null
}
return graphql.MarshalString(obj.Format(time.RFC3339))
}

func (ec *executionContext) unmarshalInputUUID(ctx context.Context, obj interface{}) (uuid.UUID, error) {
str, ok := obj.(string)
if !ok {
return uuid.Nil, fmt.Errorf("UUID must be a string")
}
return uuid.Parse(str)
}

func (ec *executionContext) _UUID(ctx context.Context, sel interface{}, obj *uuid.UUID) graphql.Marshaler {
if obj == nil {
return graphql.Null
}
return graphql.MarshalString(obj.String())
}
