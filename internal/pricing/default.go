package pricing

import "sync"

var (
	defaultOnce      sync.Once
	defaultEstimator Estimator
	defaultErr       error
)

func DefaultEstimator() Estimator {
	defaultOnce.Do(func() {
		catalog, err := LoadBundled()
		if err != nil {
			defaultErr = err
			defaultEstimator = NewHolder(emptyCatalog())
			return
		}
		defaultEstimator = catalog
	})
	return defaultEstimator
}

func DefaultError() error {
	defaultOnce.Do(func() {
		catalog, err := LoadBundled()
		if err != nil {
			defaultErr = err
			defaultEstimator = NewHolder(emptyCatalog())
			return
		}
		defaultEstimator = catalog
	})
	return defaultErr
}
