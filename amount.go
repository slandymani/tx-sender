package main

import (
	"math/big"
	"strings"

	"gitlab.com/distributed_lab/logan/v3/errors"
)

const Decimals = 18

type Amount struct {
	amount   *big.Int
	decimals uint8
}

func AmountFromInt(amount *big.Int) *Amount {
	newAmount := new(big.Int)

	return &Amount{
		amount:   newAmount.Set(amount),
		decimals: Decimals,
	}
}

func AmountFromString(amountStr string) (*Amount, error) {
	var f, o, r big.Rat

	_, ok := f.SetString(amountStr)
	if !ok {
		return nil, errors.Errorf("cannot parse amount: %s", amountStr)
	}

	one := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(Decimals)), nil)
	o.SetInt(one)
	r.Mul(&f, &o)

	is := r.FloatString(0)
	amount := big.NewInt(0)
	amount.SetString(is, 10)

	cmp := new(big.Rat)
	cmp.SetString("0")
	if f.Cmp(cmp) != 0 && amount.Int64() == 0 {
		return nil, errors.New("invalid input")
	}
	if amount.Int64() == 0 {
		amount.SetInt64(0)
	}

	return &Amount{
		amount:   amount,
		decimals: Decimals,
	}, nil
}

// Add converts amounts with different decimals to default decimals (18)
func (z *Amount) Add(x, y *Amount) *Amount {
	xAdd := new(Amount)
	yAdd := new(Amount)

	var decimals uint8
	if x.decimals != y.decimals {
		xAdd = x.ToDefaultDecimals()
		yAdd = y.ToDefaultDecimals()
		decimals = 18
	} else {
		xAdd = x
		yAdd = y
		decimals = x.decimals
	}

	z.amount.Add(xAdd.amount, yAdd.amount)

	return &Amount{
		amount:   z.amount,
		decimals: decimals,
	}
}

func (z *Amount) Sub(x, y *Amount) *Amount {
	xAdd := new(Amount)
	yAdd := new(Amount)

	var decimals uint8
	if x.decimals != y.decimals {
		xAdd = x.ToDefaultDecimals()
		yAdd = y.ToDefaultDecimals()
		decimals = 18
	} else {
		xAdd = x
		yAdd = y
		decimals = x.decimals
	}

	z.amount.Sub(xAdd.amount, yAdd.amount)

	return &Amount{
		amount:   z.amount,
		decimals: decimals,
	}
}

// Mul converts amounts with different decimals to default decimals (18)
func Mul(x, y *Amount) *Amount {
	xMul := new(Amount)
	yMul := new(Amount)

	var decimals uint8
	if x.decimals != y.decimals {
		xMul = x.ToDefaultDecimals()
		yMul = y.ToDefaultDecimals()
		decimals = 18
	} else {
		xMul = x
		yMul = y
		decimals = x.decimals
	}

	one := new(big.Int)
	one.Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)

	z := AmountFromInt(big.NewInt(0))
	z.amount.Mul(xMul.amount, yMul.amount)
	z.amount.Div(z.amount, one)

	return &Amount{
		amount:   z.amount,
		decimals: decimals,
	}
}

// Cmp compares x and y and returns:
//  1. -1 if x <  y
//  2. 0 if x == y
//  3. +1 if x >  y
func (z *Amount) Cmp(y *Amount) int {
	zCmp := new(Amount)
	yCmp := new(Amount)

	if z.decimals != y.decimals {
		zCmp = z.ToDefaultDecimals()
		yCmp = y.ToDefaultDecimals()
	} else {
		zCmp = z
		yCmp = y
	}

	return zCmp.amount.Cmp(yCmp.amount)
}

func (z *Amount) Int() *big.Int {
	return z.amount
}

// String returns value considering decimals
func (z *Amount) String() string {
	var f, o, r big.Rat

	one := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(z.decimals)), nil)
	f.SetInt(z.amount)
	o.SetInt(one)
	r.Quo(&f, &o)

	if z.amount.Int64() == 0 {
		return "0"
	}

	if strings.Contains(r.FloatString(int(z.decimals)), ".") {
		return strings.TrimRight(strings.TrimRight(r.FloatString(int(z.decimals)), "0"), ".")
	}

	return r.FloatString(int(z.decimals))
}

func (z *Amount) ToDefaultDecimals() *Amount {
	am, _ := AmountFromString(z.String())
	return am
}

func (z *Amount) IsZero() bool {
	return len(z.amount.Bits()) == 0
}
