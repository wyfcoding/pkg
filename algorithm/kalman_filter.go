package algorithm

// KalmanFilter 一维卡尔曼滤波器
// 用于平滑噪声数据，估算系统的真实状态。
// 适用场景：销量预测、价格走势平滑、传感器数据过滤。
type KalmanFilter struct {
	q float64 // 过程噪声协方差 (Process Noise Covariance)
	r float64 // 测量噪声协方差 (Measurement Noise Covariance)
	x float64 // 估算值 (Value)
	p float64 // 估算协方差 (Estimation Error Covariance)
	k float64 // 卡尔曼增益 (Kalman Gain)
}

// NewKalmanFilter 创建滤波器
// q: 过程噪声，越小系统越稳定（但也越迟钝）
// r: 测量噪声，越大系统越信任历史估算值
// initialValue: 初始值
func NewKalmanFilter(q, r, initialValue float64) *KalmanFilter {
	return &KalmanFilter{
		q: q,
		r: r,
		x: initialValue,
		p: 1.0, // 初始协方差不为0即可
	}
}

// Update 观测到一个新值，返回过滤后的预测值
func (f *KalmanFilter) Update(measurement float64) float64 {
	// 1. 预测阶段
	// p = p + q
	f.p = f.p + f.q

	// 2. 更新阶段
	// 计算卡尔曼增益 k = p / (p + r)
	f.k = f.p / (f.p + f.r)
	// 更新估算值 x = x + k * (measurement - x)
	f.x = f.x + f.k*(measurement-f.x)
	// 更新协方差 p = (1 - k) * p
	f.p = (1 - f.k) * f.p

	return f.x
}

// CurrentValue 获取当前估算值
func (f *KalmanFilter) CurrentValue() float64 {
	return f.x
}

// GetConfidence 获取估算的相对置信度 (0.0 到 1.0)
// 基于协方差 p 计算。p 越小，置信度越高。
func (f *KalmanFilter) GetConfidence() float64 {
	// 简单的非线性映射，实际可根据业务方差分布调整
	if f.p <= 0 {
		return 1.0
	}
	conf := 1.0 / (1.0 + f.p)
	if conf > 1.0 {
		return 1.0
	}
	return conf
}
