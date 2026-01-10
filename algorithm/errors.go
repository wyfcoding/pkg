package algorithm

import "errors"

var (
	// ErrEmptyData 输入数据为空。
	ErrEmptyData = errors.New("empty data")
	// ErrInvalidInput 输入格式错误。
	ErrInvalidInput = errors.New("invalid input")
	// ErrZeroVariance 方差为零。
	ErrZeroVariance = errors.New("zero variance")
	// ErrInvalidOptionType 无效的期权类型。
	ErrInvalidOptionType = errors.New("invalid option type")
	// ErrInvalidConfig 配置错误。
	ErrInvalidConfig = errors.New("tick and wheelSize must be positive")
	// ErrInvalidDelay 延迟时间错误。
	ErrInvalidDelay = errors.New("delay must be non-negative")
	// ErrNotRunning 时间轮未运行。
	ErrNotRunning = errors.New("timing wheel is not running")
	// ErrDimMismatch 维度不匹配.
	ErrDimMismatch = errors.New("dimension mismatch")
	// ErrNotSquare 不是方阵.
	ErrNotSquare = errors.New("matrix must be square")
	// ErrNotPositiveDefinite 不是正定矩阵.
	ErrNotPositiveDefinite = errors.New("matrix is not positive definite")
	// ErrMathConvergence 数学计算未收敛。
	ErrMathConvergence = errors.New("math convergence failed")
	// ErrInvalidProbabilities 概率之和不为 1。
	ErrInvalidProbabilities = errors.New("probabilities must sum to 1.0")
	// ErrCapacityPowerOf2 容量必须是 2 的幂。
	ErrCapacityPowerOf2 = errors.New("capacity must be a power of 2")
	// ErrCapacityTooSmall 容量太小。
	ErrCapacityTooSmall = errors.New("capacity must be at least 2")
)
