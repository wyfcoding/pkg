package xerrors

// 跨领域统一错误码：确保 100+ 服务响应一致性。

const (
	// 通用错误
	CodeInternal = 1000
	CodeInvalidArg = 1001
	CodeUnauthenticated = 1002
	CodePermissionDenied = 1003

	// 电商特化
	CodeOrderConflict = 2001
	CodeStockOut = 2002
	CodeAuctionClosed = 2003

	// 金融特化
	CodeRiskLimitExceeded = 3001
	CodeInsufficientBalance = 3002
	CodeFixSessionDown = 3003
	CodeMarketNotOpen = 3004
)
