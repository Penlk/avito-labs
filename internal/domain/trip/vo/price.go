package vo

type Price struct {
	rubles int64
}

func NewPrice(price int64) (Price, error) {
	if price < 0 {
		return Price{}, ValidationError{Field: "price", Reason: "must be non negative"}
	}

	return Price{rubles: price}, nil
}

func (p Price) Rubles() int64 {
	return p.rubles
}
