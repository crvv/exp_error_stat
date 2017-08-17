package main

import (
	"github.com/jackc/pgx"
	"log"
	"math"
)

var conn *pgx.Conn

func init() {
	var err error
	conn, err = pgx.Connect(pgx.ConnConfig{
		User:     "postgres",
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
  asm NUMERIC(24, 20),
  taylor NUMERIC(24, 20),
  tail300 NUMERIC(24, 20),
  tail400 NUMERIC(24, 20),
  tail500 NUMERIC(24, 20),
  tail700 NUMERIC(24, 20),
  tail800 NUMERIC(24, 20),
  tail900 NUMERIC(24, 20),
  tail996 NUMERIC(24, 20),
  exact NUMERIC(24, 20)
)`)
	if err != nil {
		log.Fatal(err)
	}
	insert(tx, 1)
	x := 1.0 / (1 << 28)
	for x < 0.8 {
		x *= 1.00001234
		insert(tx, x)
	}
	_, err = tx.Exec("UPDATE error_stat SET exact = exp(x)")
	if err != nil {
		log.Fatal(err)
	}
	rows, err := tx.Query(`SELECT
  max(abs(tail300 - exact)/exact)*1e16 AS tail300_error,
  max(abs(tail400 - exact)/exact)*1e16 AS tail400_error,
  max(abs(tail500 - exact)/exact)*1e16 AS tail500_error,
  max(abs(freebsd - exact)/exact)*1e16 AS freebsd_error,
  max(abs(sixth - exact)/exact)*1e16 AS sixth_error,
  max(abs(tail700 - exact)/exact)*1e16 AS tail700_error,
  max(abs(tail800 - exact)/exact)*1e16 AS tail800_error,
  max(abs(tail900 - exact)/exact)*1e16 AS tail900_error,
  max(abs(tail996 - exact)/exact)*1e16 AS tail996_error,
  max(abs(asm - exact)/exact)*1e16 AS asm_error,
  max(abs(taylor - exact)/exact)*1e16 AS taylor_error
FROM error_stat`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tail300, tail400, tail500, freebsd, sixth, tail700, tail800, tail900, tail996, asm, taylor float64
		err = rows.Scan(&tail300, &tail400, &tail500, &freebsd, &sixth, &tail700, &tail800, &tail900, &tail996, &asm, &taylor)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("tail300", tail300)
		log.Println("tail400", tail400)
		log.Println("tail500", tail500)
		log.Println("freebsd", freebsd)
		log.Println("1/6  p1", sixth)
		log.Println("tail700", tail700)
		log.Println("tail800", tail800)
		log.Println("tail900", tail900)
		log.Println("tail996", tail996)
		log.Println("x64 asm", asm)
		log.Println(" taylor", taylor)
	}
}

func insert(tx *pgx.Tx, x float64) {
	sql := `INSERT INTO error_stat (x, freebsd, sixth, asm, taylor, tail300, tail400, tail500, tail700, tail800, tail900, tail996) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	_, err := tx.Exec(sql, x, freebsd(x), sixth(x), asm(x), taylor(x), t300(x), t400(x), t500(x), t700(x), t800(x), t900(x), t996(x))
	if err != nil {
		log.Fatal(err)
	}
}

const (
	tail300P1 = 0.16666666666666300
	tail400P1 = 0.16666666666666400
	tail500P1 = 0.16666666666666500
	sixthP1   = 0.16666666666666666
	tail700P1 = 0.16666666666666700
	tail800P1 = 0.16666666666666800
	tail900P1 = 0.16666666666666900
	tail996P1 = 0.16666666666666996
	P1        = 0.16666666666666602
	P2        = -2.77777777770155933842e-03
	P3        = 6.61375632143793436117e-05
	P4        = -1.65339022054652515390e-06
	P5        = 4.13813679705723846039e-08
	taylorP1  = 1.0 / 6.0
	taylorP2  = -1.0 / 360
	taylorP3  = 1.0 / 15120
	taylorP4  = -1.0 / 604800
	taylorP5  = 1.0 / 23950080
)

var asm = math.Exp
var sixth = genExp(sixthP1, P2, P3, P4, P5)
var freebsd = genExp(P1, P2, P3, P4, P5)
var taylor = genExp(taylorP1, taylorP2, taylorP3, taylorP4, taylorP5)
var t300 = genExp(tail300P1, P2, P3, P4, P5)
var t400 = genExp(tail400P1, P2, P3, P4, P5)
var t500 = genExp(tail500P1, P2, P3, P4, P5)
var t700 = genExp(tail700P1, P2, P3, P4, P5)
var t800 = genExp(tail800P1, P2, P3, P4, P5)
var t900 = genExp(tail900P1, P2, P3, P4, P5)
var t996 = genExp(tail996P1, P2, P3, P4, P5)

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
