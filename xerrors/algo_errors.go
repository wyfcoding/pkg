package xerrors

var (
	// ErrEmptyData 输入数据为空。
	ErrEmptyData = New(ErrInvalidArg, 400001, "empty data", "input data must not be empty", nil)
	// ErrInvalidInput 输入格式错误。
	ErrInvalidInput = New(ErrInvalidArg, 400002, "invalid input", "check your input parameters", nil)
	// ErrZeroVariance 方差为零。
	ErrZeroVariance = New(ErrInvalidArg, 400003, "zero variance", "variance is zero, cannot proceed with calculation", nil)
	// ErrInvalidOptionType 无效的期权类型。
	ErrInvalidOptionType = New(ErrInvalidArg, 400004, "invalid option type", "supported types: call, put", nil)
	// ErrInvalidConfig 配置错误。
	ErrInvalidConfig = New(ErrInvalidArg, 400005, "invalid config", "tick and wheelSize must be positive", nil)
	// ErrInvalidDelay 延迟时间错误。
	ErrInvalidDelay = New(ErrInvalidArg, 400006, "invalid delay", "delay must be non-negative", nil)
	// ErrDimMismatch 维度不匹配.
	ErrDimMismatch = New(ErrInvalidArg, 400007, "dimension mismatch", "matrix or vector dimensions do not match", nil)
	// ErrNotSquare 不是方阵.
	ErrNotSquare = New(ErrInvalidArg, 400008, "matrix must be square", "input matrix is not square", nil)
	// ErrNotPositiveDefinite 不是正定矩阵.
	ErrNotPositiveDefinite = New(ErrInvalidArg, 400009, "matrix is not positive definite", "input matrix must be positive definite", nil)
	// ErrInvalidProbabilities 概率之和不为 1。
	ErrInvalidProbabilities = New(ErrInvalidArg, 400010, "probabilities must sum to 1.0", "check input probability distribution", nil)
	// ErrCapacityPowerOf2 容量必须是 2 的幂。
	ErrCapacityPowerOf2 = New(ErrInvalidArg, 400011, "capacity must be a power of 2", "capacity constraint violated", nil)
	// ErrCapacityTooSmall 容量太小。
	ErrCapacityTooSmall = New(ErrInvalidArg, 400012, "capacity must be at least 2", "minimum capacity requirement not met", nil)
	// ErrInsufficientLayers 神经网络层数不足。
	ErrInsufficientLayers = New(ErrInvalidArg, 400013, "insufficient layers", "neural network must have at least 2 layers", nil)
	// ErrInvalidAlpha EWMA 平滑系数错误。
	ErrInvalidAlpha = New(ErrInvalidArg, 400014, "invalid alpha", "alpha must be in range (0, 1]", nil)
	// ErrDimMismatchBounds 线性规划维度不匹配。
	ErrDimMismatchBounds = New(ErrInvalidArg, 400015, "dimension mismatch bounds", "constraints and objective function dimensions do not match", nil)
	// ErrInvalidEpsilonDelta 无效的 epsilon 或 delta 参数。
	ErrInvalidEpsilonDelta = New(ErrInvalidArg, 400016, "invalid epsilon or delta", "parameters must be between 0 and 1", nil)
	// ErrCapacityRequired 需要指定容量。
	ErrCapacityRequired = New(ErrInvalidArg, 400017, "capacity required", "initial capacity must be provided", nil)
	// ErrEmptyPoints 点集不能为空。
	ErrEmptyPoints = New(ErrInvalidArg, 400018, "empty points", "input point set must not be empty", nil)
	// ErrNotRunning 时间轮未运行。
	ErrNotRunning = New(ErrInternal, 500001, "timing wheel is not running", "start the timing wheel before adding tasks", nil)
	// ErrMathConvergence 数学计算未收敛。
	ErrMathConvergence = New(ErrInternal, 500002, "math convergence failed", "algorithm failed to converge", nil)
	// ErrUnboundedProblem 线性规划无界。
	ErrUnboundedProblem = New(ErrInternal, 500005, "unbounded problem", "linear programming problem is unbounded", nil)
	// ErrBufferFull 缓冲区已满。
	ErrBufferFull = New(ErrInternal, 500006, "buffer full", "queue or buffer has reached its capacity", nil)
	// ErrBufferEmpty 缓冲区为空。
	ErrBufferEmpty = New(ErrInternal, 500007, "buffer empty", "queue or buffer is empty", nil)
)
