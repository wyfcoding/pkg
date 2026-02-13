package fix

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	SOH = "\x01"
)

type Message struct {
	Fields map[int]string
	Tags   []int
}

func NewMessage() *Message {
	return &Message{
		Fields: make(map[int]string),
		Tags:   make([]int, 0),
	}
}

func (m *Message) Set(tag int, value string) {
	if _, ok := m.Fields[tag]; !ok {
		m.Tags = append(m.Tags, tag)
	}
	m.Fields[tag] = value
}

func (m *Message) Get(tag int) string {
	return m.Fields[tag]
}

func (m *Message) GetInt(tag int) (int, error) {
	val := m.Fields[tag]
	if val == "" {
		return 0, fmt.Errorf("tag %d not found", tag)
	}
	var result int
	for _, c := range val {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result, nil
}

func (m *Message) GetFloat(tag int) (float64, error) {
	val := m.Fields[tag]
	if val == "" {
		return 0, fmt.Errorf("tag %d not found", tag)
	}
	var result float64
	var decimal float64 = 1
	var hasDecimal bool
	for _, c := range val {
		if c == '.' {
			hasDecimal = true
			continue
		}
		if c >= '0' && c <= '9' {
			if hasDecimal {
				decimal *= 10
				result = result + float64(c-'0')/decimal
			} else {
				result = result*10 + float64(c-'0')
			}
		}
	}
	return result, nil
}

func (m *Message) Has(tag int) bool {
	_, ok := m.Fields[tag]
	return ok
}

func (m *Message) Remove(tag int) {
	delete(m.Fields, tag)
	for i, t := range m.Tags {
		if t == tag {
			m.Tags = append(m.Tags[:i], m.Tags[i+1:]...)
			break
		}
	}
}

func Parse(data []byte) (*Message, error) {
	msg := NewMessage()
	length := len(data)
	start := 0

	for start < length {
		equalIdx := -1
		for i := start; i < length; i++ {
			if data[i] == '=' {
				equalIdx = i
				break
			}
		}
		if equalIdx == -1 {
			break
		}

		tag := 0
		for i := start; i < equalIdx; i++ {
			if data[i] >= '0' && data[i] <= '9' {
				tag = tag*10 + int(data[i]-'0')
			}
		}

		valueStart := equalIdx + 1
		sohIdx := -1
		for i := valueStart; i < length; i++ {
			if data[i] == 1 {
				sohIdx = i
				break
			}
		}
		if sohIdx == -1 {
			sohIdx = length
		}

		value := string(data[valueStart:sohIdx])
		msg.Set(tag, value)

		start = sohIdx + 1
	}

	return msg, nil
}

func (m *Message) String() string {
	var buf strings.Builder
	sort.Ints(m.Tags)
	for _, tag := range m.Tags {
		buf.WriteString(fmt.Sprintf("%d=%s%s", tag, m.Fields[tag], SOH))
	}
	return buf.String()
}

func (m *Message) Bytes() []byte {
	return []byte(m.String())
}

func (m *Message) MsgType() string {
	return m.Get(TagMsgType)
}

func (m *Message) IsAdmin() bool {
	msgType := m.MsgType()
	return msgType == MsgTypeHeartbeat ||
		msgType == MsgTypeTestRequest ||
		msgType == MsgTypeResendRequest ||
		msgType == MsgTypeReject ||
		msgType == MsgTypeSequenceReset ||
		msgType == MsgTypeLogout ||
		msgType == MsgTypeLogon
}

func (m *Message) IsApplication() bool {
	return !m.IsAdmin()
}

const (
	TagBeginString   = 8
	TagBodyLength    = 9
	TagMsgType       = 35
	TagSenderCompID  = 49
	TagTargetCompID  = 56
	TagMsgSeqNum     = 34
	TagCheckSum      = 10
	TagSendingTime   = 52
	TagPossDupFlag   = 43
	TagOrigSendingTime = 122
	TagLastMsgSeqNumProcessed = 369

	TagClOrdID      = 11
	TagOrderID      = 37
	TagSymbol       = 55
	TagSide         = 54
	TagOrderQty     = 38
	TagPrice        = 44
	TagOrdType      = 40
	TagTimeInForce  = 59
	TagTransactTime = 60
	TagExecType     = 150
	TagOrdStatus    = 39
	TagLeavesQty    = 151
	TagCumQty       = 14
	TagAvgPx        = 6
	TagText         = 58
	TagAccount      = 1
	TagCurrency     = 15
	TagHandlInst    = 21
	TagSecurityType = 167
	TagMaturityMonthYear = 200
	TagPutOrCall    = 201
	TagStrikePrice  = 202
	TagSettlmntTyp  = 63
	TagFutSettDate  = 64
	TagStopPx       = 99
	TagMinQty       = 110
	TagMaxFloor     = 111
	TagExDestination = 100
	TagProcessCode  = 81
	TagPrevClosePx  = 140
	TagLocateReqd   = 114
	TagNoTradingSessions = 386
	TagTradingSessionID = 336

	TagExecID       = 17
	TagExecInst     = 18
	TagLastPx       = 31
	TagLastShares   = 32
	TagExecTransType = 20
	TagExecRefID    = 19
	TagOrderCapacity = 528
	TagRule80A      = 47

	TagReportToExch = 113
	TagLocateBroker = 114

	TagOrigClOrdID  = 41
	TagCxlRejReason = 102
	TagCxlRejResponseTo = 434

	TagListID       = 66
	TagListSeqNo    = 67
	TagTotNoOrders  = 68
	TagContingencyType = 1385

	TagNoPartyIDs   = 453
	TagPartyID      = 448
	TagPartyIDSource = 447
	TagPartyRole    = 452

	TagEncryptMethod = 98
	TagHeartBtInt   = 108
	TagResetSeqNumFlag = 141
	TagNextExpectedMsgSeqNum = 789
	TagMaxMessageSize = 383
	TagTestMessageIndicator = 464
	TagUsername     = 553
	TagPassword     = 554
	TagNewPassword  = 925
	TagSessionStatus = 1409
	TagDefaultApplVerID = 1137
	TagDefaultCstmApplVerID = 1138

	TagRefSeqNum    = 45
	TagRefTagID     = 371
	TagRefMsgType   = 372
	TagSessionRejectReason = 373
	TagTextEncoding = 369

	TagGapFillFlag  = 123
	TagNewSeqNo     = 36

	TagTestReqID    = 112

	TagBeginSeqNo   = 7
	TagEndSeqNo     = 16

	TagMDReqID      = 262
	TagSubscriptionRequestType = 263
	TagMarketDepth  = 264
	TagMDUpdateType = 265
	TagAggregatedBook = 266
	TagNoMDEntryTypes = 267
	TagMDEntryType  = 269
	TagNoRelatedSym = 146
	TagSymbolSfx    = 65
	TagSecurityID   = 48
	TagSecurityIDSource = 22
	TagNoUnderlyings = 711
	TagUnderlyingSymbol = 311
	TagUnderlyingSecurityID = 309
	TagUnderlyingSecurityIDSource = 305
	TagNoLegs       = 555
	TagLegSymbol    = 600
	TagLegSecurityID = 602
	TagLegSecurityIDSource = 603
	TagLegSide      = 604

	TagMDReqRejReason = 281
	TagMDRejReasonText = 282

	TagMDUpdateAction = 279
	TagDeleteReason  = 285
	TagMDEntryID     = 278
	TagMDUpdateActionMDEntryType = 279
	TagMDEntryPx     = 270
	TagMDEntrySize   = 271
	TagMDEntryDate   = 272
	TagMDEntryTime   = 273
	TagTickDirection = 274
	TagMDMkt         = 275
	TagQuoteCondition = 276
	TagTradeCondition = 277
	TagMDEntryOriginator = 282
	TagLocationID    = 283
	TagDeskID        = 284
	TagOpenCloseSettlFlag = 286
	TagSellerDays    = 287
	TagMDEntryBuyer  = 288
	TagMDEntrySeller = 289
	TagNumberOfOrders = 296
	TagMDEntryPositionNo = 293
	TagScope         = 546
	TagPriceDelta    = 811

	TagSecurityStatusReqID = 324
	TagSecurityStatusID = 325
	TagSecurityTradingStatus = 326
	TagHaltReasonChar = 327
	TagInViewOfCommon = 328
	TagNetChgPrevDay = 451

	TagTradingSessionSubID = 625
	TagTradSesMethod = 338
	TagTradSesMode   = 339
	TagTradSesStatus = 340
	TagTradSesStartTime = 341
	TagTradSesOpenTime = 342
	TagTradSesPreCloseTime = 343
	TagTradSesCloseTime = 344
	TagTradSesEndTime = 345
	TagTradSesIntermission = 1031

	TagBidPx         = 132
	TagOfferPx       = 133
	TagBidSize       = 134
	TagOfferSize     = 135
	TagValidUntilTime = 62
	TagBidSpotRate   = 188
	TagOfferSpotRate = 190
	TagBidForwardPoints = 189
	TagOfferForwardPoints = 191
	TagMidPx         = 631
	TagBidYield      = 632
	TagMidYield      = 633
	TagOfferYield    = 634
	TagClearingBusinessDate = 715

	TagSettlDate    = 64
	TagSettlDate2   = 193
	TagOrderQty2    = 192
	TagCurrencyCode = 15
	TagSettlCurrency = 120
	TagSettlCurrAmt = 119
	TagSettlCurrFxRate = 156
	TagSettlCurrFxRateCalc = 157
	TagSettlType    = 63
	TagSettlInstMode = 160
	TagSettlInstID  = 162
	TagSettlInstTransType = 163
	TagSettlInstRefID = 164
	TagSettlDeliveryType = 172
	TagStandInstDbType = 169
	TagStandInstDbName = 170
	TagStandInstDbID = 171

	TagAllocID      = 70
	TagAllocTransType = 71
	TagAllocType    = 626
	TagNoAllocs     = 78
	TagAllocAccount = 79
	TagAllocQty     = 80
	TagAllocPrice   = 366
	TagTradeDate    = 75
	TagNoOrders     = 73

	TagPositionEffect = 77
	TagCoveredOrUncovered = 203
	TagCustomerOrFirm = 204
	TagSelfMatchPreventionID = 792
	TagSelfMatchPreventionInstruction = 800
)

const (
	MsgTypeHeartbeat       = "0"
	MsgTypeTestRequest     = "1"
	MsgTypeResendRequest   = "2"
	MsgTypeReject          = "3"
	MsgTypeSequenceReset   = "4"
	MsgTypeLogout          = "5"
	MsgTypeLogon           = "A"
	MsgTypeNewOrderSingle  = "D"
	MsgTypeExecutionReport = "8"
	MsgTypeOrderCancelRequest = "F"
	MsgTypeOrderCancelReject = "9"
	MsgTypeOrderCancelReplaceRequest = "G"
	MsgTypeOrderStatusRequest = "H"
	MsgTypeNewOrderList = "E"
	MsgTypeOrderMassCancelRequest = "q"
	MsgTypeOrderMassCancelReport = "r"
	MsgTypeOrderMassStatusRequest = "AF"
	MsgTypeNewOrderCross = "s"
	MsgTypeCrossOrderCancelReplaceRequest = "t"
	MsgTypeCrossOrderCancelRequest = "u"
	MsgTypeSecurityDefinitionRequest = "c"
	MsgTypeSecurityDefinition = "d"
	MsgTypeSecurityStatusRequest = "e"
	MsgTypeSecurityStatus = "f"
	MsgTypeTradingSessionStatusRequest = "g"
	MsgTypeTradingSessionStatus = "h"
	MsgTypeMarketDataRequest = "V"
	MsgTypeMarketDataSnapshotFullRefresh = "W"
	MsgTypeMarketDataIncrementalRefresh = "X"
	MsgTypeMarketDataRequestReject = "Y"
	MsgTypeQuoteRequest = "R"
	MsgTypeQuote = "S"
	MsgTypeQuoteCancel = "Z"
	MsgTypeQuoteStatusRequest = "a"
	MsgTypeQuoteStatusReport = "AI"
	MsgTypeAllocationInstruction = "J"
	MsgTypeAllocationInstructionAck = "P"
	MsgTypeAllocationReport = "AS"
	MsgTypeAllocationReportAck = "AT"
	MsgTypePositionReport = "AP"
	MsgTypeRequestForPositions = "AN"
	MsgTypeRequestForPositionsAck = "AO"
	MsgTypeCollateralAssignment = "AY"
	MsgTypeCollateralRequest = "AX"
	MsgTypeCollateralResponse = "AZ"
	MsgTypeCollateralReport = "BA"
	MsgTypeCollateralInquiry = "BB"
	MsgTypeNetworkStatusRequest = "BC"
	MsgTypeNetworkStatusResponse = "BD"
	MsgTypeUserRequest = "BE"
	MsgTypeUserResponse = "BF"
)

const (
	OrdTypeMarket = "1"
	OrdTypeLimit = "2"
	OrdTypeStop = "3"
	OrdTypeStopLimit = "4"
	OrdTypeMarketOnClose = "5"
	OrdTypeWithOrWithout = "6"
	OrdTypeLimitOrBetter = "7"
	OrdTypeLimitWithOrWithout = "8"
	OrdTypeOnBasis = "9"
	OrdTypeOnClose = "A"
	OrdTypeLimitOnClose = "B"
	OrdTypeForexMarket = "C"
	OrdTypePreviouslyQuoted = "D"
	OrdTypePreviouslyIndicated = "E"
	OrdTypeForexLimit = "F"
	OrdTypeForexSwap = "G"
	OrdTypeForexPreviouslyQuoted = "H"
	OrdTypeFunari = "I"
	OrdTypeMarketIfTouched = "J"
	OrdTypeMarketWithLeftOverAsLimit = "K"
	OrdTypePreviousFundValuationPoint = "L"
	OrdTypeNextFundValuationPoint = "M"
	OrdTypePegged = "P"
	OrdTypeCounterOrderSelection = "Q"
)

const (
	SideBuy = "1"
	SideSell = "2"
	SideBuyMinus = "3"
	SideSellPlus = "4"
	SideSellShort = "5"
	SideSellShortExempt = "6"
	SideUndisclosed = "7"
	SideCross = "8"
	SideCrossShort = "9"
	SideCrossShortExempt = "A"
	SideAsDefined = "B"
	SideOpposite = "C"
	SideSubscribe = "D"
	SideRedeem = "E"
	SideLend = "F"
	SideBorrow = "G"
)

const (
	TimeInForceDay = "0"
	TimeInForceGoodTillCancel = "1"
	TimeInForceAtTheOpening = "2"
	TimeInForceImmediateOrCancel = "3"
	TimeInForceFillOrKill = "4"
	TimeInForceGoodTillCrossing = "5"
	TimeInForceGoodTillDate = "6"
	TimeInForceAtTheClose = "7"
)

const (
	OrdStatusNew = "0"
	OrdStatusPartiallyFilled = "1"
	OrdStatusFilled = "2"
	OrdStatusDoneForDay = "3"
	OrdStatusCanceled = "4"
	OrdStatusReplaced = "5"
	OrdStatusPendingCancel = "6"
	OrdStatusStopped = "7"
	OrdStatusRejected = "8"
	OrdStatusSuspended = "9"
	OrdStatusPendingNew = "A"
	OrdStatusCalculated = "B"
	OrdStatusExpired = "C"
	OrdStatusAcceptedForBidding = "D"
	OrdStatusPendingReplace = "E"
)

const (
	ExecTypeNew = "0"
	ExecTypePartialFill = "1"
	ExecTypeFill = "2"
	ExecTypeDoneForDay = "3"
	ExecTypeCanceled = "4"
	ExecTypeReplace = "5"
	ExecTypePendingCancel = "6"
	ExecTypeStopped = "7"
	ExecTypeRejected = "8"
	ExecTypeSuspended = "9"
	ExecTypePendingNew = "A"
	ExecTypeCalculated = "B"
	ExecTypeExpired = "C"
	ExecTypeRestated = "D"
	ExecTypePendingReplace = "E"
	ExecTypeTrade = "F"
	ExecTypeTradeCorrect = "G"
	ExecTypeTradeCancel = "H"
	ExecTypeOrderStatus = "I"
	ExecTypeDoneForDayRest = "J"
)

const (
	MDEntryTypeBid = "0"
	MDEntryTypeOffer = "1"
	MDEntryTypeTrade = "2"
	MDEntryTypeIndexValue = "3"
	MDEntryTypeOpeningPrice = "4"
	MDEntryTypeClosingPrice = "5"
	MDEntryTypeSettlementPrice = "6"
	MDEntryTypeTradingHigh = "7"
	MDEntryTypeTradingLow = "8"
	MDEntryTypeVWAP = "9"
)

const (
	SubscriptionRequestTypeSnapshot = "0"
	SubscriptionRequestTypeSubscribe = "1"
	SubscriptionRequestTypeDisablePreviousSnapshot = "2"
)

func BuildLogon(heartBtInt int, senderCompID, targetCompID string, seqNum int64) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeLogon)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagHeartBtInt, fmt.Sprintf("%d", heartBtInt))
	msg.Set(TagEncryptMethod, "0")
	msg.Set(TagSendingTime, timeNowUTC())
	return msg
}

func BuildLogout(senderCompID, targetCompID string, seqNum int64, text string) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeLogout)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	if text != "" {
		msg.Set(TagText, text)
	}
	return msg
}

func BuildHeartbeat(senderCompID, targetCompID string, seqNum int64, testReqID string) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeHeartbeat)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	if testReqID != "" {
		msg.Set(TagTestReqID, testReqID)
	}
	return msg
}

func BuildTestRequest(senderCompID, targetCompID string, seqNum int64, testReqID string) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeTestRequest)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagTestReqID, testReqID)
	return msg
}

func BuildResendRequest(senderCompID, targetCompID string, seqNum int64, beginSeqNo, endSeqNo int) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeResendRequest)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagBeginSeqNo, fmt.Sprintf("%d", beginSeqNo))
	msg.Set(TagEndSeqNo, fmt.Sprintf("%d", endSeqNo))
	return msg
}

func BuildSequenceReset(senderCompID, targetCompID string, seqNum int64, newSeqNo int, gapFillFlag bool) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeSequenceReset)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagNewSeqNo, fmt.Sprintf("%d", newSeqNo))
	if gapFillFlag {
		msg.Set(TagGapFillFlag, "Y")
	}
	return msg
}

func BuildReject(senderCompID, targetCompID string, seqNum int64, refSeqNum int, reason int, text string) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeReject)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagRefSeqNum, fmt.Sprintf("%d", refSeqNum))
	msg.Set(TagSessionRejectReason, fmt.Sprintf("%d", reason))
	if text != "" {
		msg.Set(TagText, text)
	}
	return msg
}

func BuildNewOrderSingle(clOrdID, symbol, side, ordType string, qty float64, price float64, senderCompID, targetCompID string, seqNum int64) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeNewOrderSingle)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagClOrdID, clOrdID)
	msg.Set(TagSymbol, symbol)
	msg.Set(TagSide, side)
	msg.Set(TagOrdType, ordType)
	msg.Set(TagOrderQty, fmt.Sprintf("%.8f", qty))
	if price > 0 {
		msg.Set(TagPrice, fmt.Sprintf("%.8f", price))
	}
	msg.Set(TagTimeInForce, TimeInForceDay)
	msg.Set(TagTransactTime, timeNowUTC())
	return msg
}

func BuildExecutionReport(orderID, clOrdID, execID, symbol, side, ordType, ordStatus, execType string, qty, price, leavesQty, cumQty, avgPx float64, senderCompID, targetCompID string, seqNum int64) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeExecutionReport)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagOrderID, orderID)
	msg.Set(TagClOrdID, clOrdID)
	msg.Set(TagExecID, execID)
	msg.Set(TagSymbol, symbol)
	msg.Set(TagSide, side)
	msg.Set(TagOrdType, ordType)
	msg.Set(TagOrdStatus, ordStatus)
	msg.Set(TagExecType, execType)
	msg.Set(TagOrderQty, fmt.Sprintf("%.8f", qty))
	if price > 0 {
		msg.Set(TagPrice, fmt.Sprintf("%.8f", price))
	}
	msg.Set(TagLeavesQty, fmt.Sprintf("%.8f", leavesQty))
	msg.Set(TagCumQty, fmt.Sprintf("%.8f", cumQty))
	msg.Set(TagAvgPx, fmt.Sprintf("%.8f", avgPx))
	msg.Set(TagTransactTime, timeNowUTC())
	return msg
}

func BuildOrderCancelRequest(clOrdID, origClOrdID, orderID, symbol, side string, qty float64, senderCompID, targetCompID string, seqNum int64) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeOrderCancelRequest)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagClOrdID, clOrdID)
	if origClOrdID != "" {
		msg.Set(TagOrigClOrdID, origClOrdID)
	}
	if orderID != "" {
		msg.Set(TagOrderID, orderID)
	}
	msg.Set(TagSymbol, symbol)
	msg.Set(TagSide, side)
	msg.Set(TagOrderQty, fmt.Sprintf("%.8f", qty))
	msg.Set(TagTransactTime, timeNowUTC())
	return msg
}

func BuildMarketDataRequest(mdReqID string, subscriptionType string, depth int, symbols []string, entryTypes []string, senderCompID, targetCompID string, seqNum int64) *Message {
	msg := NewMessage()
	msg.Set(TagBeginString, "FIX.4.4")
	msg.Set(TagMsgType, MsgTypeMarketDataRequest)
	msg.Set(TagSenderCompID, senderCompID)
	msg.Set(TagTargetCompID, targetCompID)
	msg.Set(TagMsgSeqNum, fmt.Sprintf("%d", seqNum))
	msg.Set(TagSendingTime, timeNowUTC())
	msg.Set(TagMDReqID, mdReqID)
	msg.Set(TagSubscriptionRequestType, subscriptionType)
	msg.Set(TagMarketDepth, fmt.Sprintf("%d", depth))
	msg.Set(TagNoRelatedSym, fmt.Sprintf("%d", len(symbols)))
	for i, sym := range symbols {
		msg.Set(TagSymbol, sym)
		_ = i
	}
	msg.Set(TagNoMDEntryTypes, fmt.Sprintf("%d", len(entryTypes)))
	for _, et := range entryTypes {
		msg.Set(TagMDEntryType, et)
	}
	return msg
}

func timeNowUTC() string {
	return timeNow().Format("20060102-15:04:05.000")
}

var timeNow = func() time.Time {
	return time.Now().UTC()
}
