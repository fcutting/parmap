package parmap

import (
	"errors"
	"fmt"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type doFunc[IN, OUT any] func(input IN) (result OUT, err error)

type data[T any] struct {
	index int
	value T
	final bool
}

type ErrMap map[int]error

func (e ErrMap) ErrJoin() error {
	errs := make([]error, len(e))

	keys := maps.Keys(e)
	slices.Sort(keys)

	for i, k := range keys {
		errs[i] = fmt.Errorf("%d: %w", k, e[k])
	}

	return errors.Join(errs...)
}

func (e ErrMap) String() string {
	return e.ErrJoin().Error()
}

func (e ErrMap) Error() string {
	return e.String()
}

func startResultActor[OUT any](doneChan chan struct{}, length int) (resultChan chan data[OUT], results []OUT) {
	results = make([]OUT, length)
	resultChan = make(chan data[OUT])

	go func() {
		for {
			select {
			case result, open := <-resultChan:
				if open {
					results[result.index] = result.value
					doneChan <- struct{}{}
				} else {
					return
				}
			}
		}
	}()

	return resultChan, results
}

func startErrActor(doneChan chan struct{}) (errChan chan data[error], erm ErrMap) {
	erm = make(ErrMap)
	errChan = make(chan data[error])

	go func() {
		for {
			select {
			case erd, open := <-errChan:
				if open {
					erm[erd.index] = erd.value
					doneChan <- struct{}{}
				} else {
					return
				}
			}
		}
	}()

	return errChan, erm
}

func startDoActor[IN, OUT any](do doFunc[IN, OUT], inputChan chan data[IN], resultChan chan data[OUT], errChan chan data[error]) {
	go func() {
		for {
			select {
			case input, open := <-inputChan:
				if open {
					result, err := do(input.value)

					if err != nil {
						errChan <- data[error]{
							index: input.index,
							value: err,
						}

						break
					}

					resultChan <- data[OUT]{
						index: input.index,
						value: result,
					}
				} else {
					return
				}
			}
		}
	}()
}

func Do[IN, OUT any](inputs []IN, do doFunc[IN, OUT]) (result []OUT, err ErrMap) {
	inputsLen := len(inputs)
	inputChan := make(chan data[IN])
	doneChan := make(chan struct{})
	defer close(doneChan)
	resultChan, results := startResultActor[OUT](doneChan, inputsLen)
	defer close(resultChan)
	errChan, erm := startErrActor(doneChan)
	defer close(errChan)

	for range inputsLen {
		startDoActor(do, inputChan, resultChan, errChan)
	}

	for i, v := range inputs {
		inputChan <- data[IN]{
			index: i,
			value: v,
		}
	}

	close(inputChan)

	for range inputsLen {
		<-doneChan
	}

	if len(erm) != 0 {
		return results, erm
	}

	return results, nil
}
