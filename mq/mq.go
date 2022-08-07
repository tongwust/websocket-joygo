package mq

type Data struct {
	T         int    `json:"t"`
	Id        string `json:"id"`
	SellCount int64  `json:"sell_count"`
	Period    string `json:"period"`
	GoodsType string `json:"goods_type"`
	Status    int    `json:"status"`
}
