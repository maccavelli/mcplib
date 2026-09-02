package selfupdate

import "context"

type noopTransformer struct{}

func (noopTransformer) Transform(context.Context, TransformRequest) error {
	return nil
}
