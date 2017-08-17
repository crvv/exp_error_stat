package main

import (
	"math"
	"github.com/jackc/pgx"
	"log"
)

var conn *pgx.Conn

func init() {
	var err error
	conn, err = pgx.Connect(pgx.ConnConfig{
		User: "postgres",
		Database: "postgres",
	})
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	tx, err := conn.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	_, err = tx.Exec(`CREATE TABLE error_stat (
  x NUMERIC(24, 20),
  freebsd NUMERIC(24, 20),
  sixth NUMERIC(24, 20),
  closest NUMERIC(24, 20),
  asm NUMERIC(24, 20),
  taylor NUMERIC(24, 20),
  exact NUMERIC(24, 20)
)`)
	if err != nil {
		log.Fatal(err)
	}
	insert(tx, 1)
	x := 1.0 / (1 << 28)
	for x < 0.8 {
		x *= 1.0001234
		insert(tx, x)
	}
	_, err = tx.Exec("UPDATE error_stat SET exact = exp(x)")
	if err != nil {
		log.Fatal(err)
	}
	rows, err := tx.Query(`SELECT
  max(abs(freebsd - exact)/exact)*1e16 AS freebsd_error,
  max(abs(sixth - exact)/exact)*1e16 AS sixth_error,
  max(abs(closest - exact)/exact)*1e16 AS closest_error,
  max(abs(asm - exact)/exact)*1e16 AS asm_error,
  max(abs(taylor - exact)/exact)*1e16 AS taylor_error
FROM error_stat`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var freebsdError, sixthError, closestError, asmError, taylorError float64
		err = rows.Scan(&freebsdError, &sixthError, &closestError, &asmError, &taylorError)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("freebsd", freebsdError,
			"sixth", sixthError,
			"closest", closestError,
			"asm", asmError,
			"taylor", taylorError)
	}
	defer rows.Close()
}

func insert(tx *pgx.Tx, x float64) {
	sql := "INSERT INTO error_stat (x, freebsd, sixth, closest, asm, taylor) VALUES ($1, $2, $3, $4, $5, $6)"
	_, err := tx.Exec(sql, x, freebsd(x), sixth(x), closest(x), asm(x), taylor(x))
	if err != nil {
		log.Fatal(err)
	}
}

const (
	P1        = 1.66666666666666019037e-01
	closestP1 = 0.16666666666666638
	tP1       = 1.0 / 6.0
	sixthP1   = 1.0 / 6.0
	P2        = -2.77777777770155933842e-03
	tP2       = -1.0 / 360
	P3        = 6.61375632143793436117e-05
	tP3       = 1.0 / 15120
	P4        = -1.65339022054652515390e-06
	tP4       = -1.0 / 604800
	P5        = 4.13813679705723846039e-08
	tP5       = 1.0 / 23950080
)

var asm = math.Exp
var sixth = genExp(sixthP1, P2, P3, P4, P5)
var freebsd = genExp(P1, P2, P3, P4, P5)
var closest = genExp(closestP1, P2, P3, P4, P5)
var taylor = genExp(tP1, tP2, tP3, tP4, tP5)

func genExp(P1, P2, P3, P4, P5 float64) func(float64) float64 {
	exp := func(x float64) float64 {
		result, ok := check(x)
		if ok {
			return result
		}
		h, l, k := reduce(x)
		return expmulti(h, l, k, P1, P2, P3, P4, P5)
	}
	return exp
}

const (
	Ln2Hi = 6.93147180369123816490e-01
	Ln2Lo = 1.90821492927058770002e-10
	Log2e = 1.44269504088896338700e+00

	Overflow  = 7.09782712893383973096e+02
	Underflow = -7.45133219101941108420e+02
	NearZero  = 1.0 / (1 << 28) // 2**-28
)

func check(x float64) (float64, bool) {
	// special cases
	switch {
	case math.IsNaN(x) || math.IsInf(x, 1):
		return x, true
	case math.IsInf(x, -1):
		return 0, true
	case x > Overflow:
		return math.Inf(1), true
	case x < Underflow:
		return 0, true
	case -NearZero < x && x < NearZero:
		return 1 + x, true
	}
	return 0, false
}
func reduce(x float64) (float64, float64, int) {
	var k int
	switch {
	case x < 0:
		k = int(Log2e*x - 0.5)
	case x > 0:
		k = int(Log2e*x + 0.5)
	}
	hi := x - float64(k)*Ln2Hi
	lo := float64(k) * Ln2Lo
	return hi, lo, k
}
func expmulti(hi, lo float64, k int, P1, P2, P3, P4, P5 float64) float64 {
	r := hi - lo
	t := r * r
	c := r - t*(P1+t*(P2+t*(P3+t*(P4+t*P5))))
	y := 1 - ((lo - (r*c)/(2-c)) - hi)
	return math.Ldexp(y, k)
}
