// Package translator provides types and functions for converting chat requests and responses between different schemas.
package translator

import "context"

// RequestTransform is a function type that converts a request payload from a source schema to a target schema.
type RequestTransform func(model string, rawJSON []byte, stream bool) []byte

// ResponseStreamTransform is a function type that converts a streaming response from a source schema to a target schema.
type ResponseStreamTransform func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []string

// ResponseNonStreamTransform is a function type that converts a non-streaming response from a source schema to a target schema.
type ResponseNonStreamTransform func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) string

// ResponseTokenCountTransform is a function type that transforms a token count from a source format to a target format.
type ResponseTokenCountTransform func(ctx context.Context, count int64) string

// ResponseTransform is a struct that groups together the functions for transforming streaming and non-streaming responses,
// as well as token counts.
type ResponseTransform struct {
	Stream     ResponseStreamTransform
	NonStream  ResponseNonStreamTransform
	TokenCount ResponseTokenCountTransform
}
