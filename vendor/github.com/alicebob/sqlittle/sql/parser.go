// Code generated by goyacc -o parser.go parser.go.y. DO NOT EDIT.

//line parser.go.y:2
package sql

import __yyfmt__ "fmt"

//line parser.go.y:2

//line parser.go.y:5
type yySymType struct {
	yys                  int
	literal              string
	identifier           string
	signedNumber         int64
	statement            interface{}
	columnNameList       []string
	columnName           string
	columnDefList        []ColumnDef
	columnDef            ColumnDef
	indexedColumnList    []IndexedColumn
	indexedColumn        IndexedColumn
	name                 string
	withoutRowid         bool
	unique               bool
	bool                 bool
	collate              string
	sortOrder            SortOrder
	columnConstraint     columnConstraint
	columnConstraintList []columnConstraint
	tableConstraint      TableConstraint
	tableConstraintList  []TableConstraint
	foreignKeyClause     ForeignKeyClause
	triggerAction        TriggerAction
	trigger              Trigger
	triggerList          []Trigger
	where                Expression
	expr                 Expression
	exprList             []Expression
	float                float64
}

const ABORT = 57346
const ACTION = 57347
const AND = 57348
const ASC = 57349
const AUTOINCREMENT = 57350
const CASCADE = 57351
const CHECK = 57352
const COLLATE = 57353
const CONFLICT = 57354
const CONSTRAINT = 57355
const CREATE = 57356
const DEFAULT = 57357
const DEFERRABLE = 57358
const DEFERRED = 57359
const DELETE = 57360
const DESC = 57361
const FAIL = 57362
const FOREIGN = 57363
const FROM = 57364
const GLOB = 57365
const IGNORE = 57366
const IN = 57367
const INDEX = 57368
const INITIALLY = 57369
const IS = 57370
const KEY = 57371
const LIKE = 57372
const MATCH = 57373
const NO = 57374
const NOT = 57375
const NULL = 57376
const ON = 57377
const OR = 57378
const PRIMARY = 57379
const REFERENCES = 57380
const REGEXP = 57381
const REPLACE = 57382
const RESTRICT = 57383
const ROLLBACK = 57384
const ROWID = 57385
const SELECT = 57386
const SET = 57387
const TABLE = 57388
const UNIQUE = 57389
const UPDATE = 57390
const WHERE = 57391
const WITHOUT = 57392
const tBare = 57393
const tLiteral = 57394
const tIdentifier = 57395
const tOperator = 57396
const tSignedNumber = 57397
const tFloat = 57398

var yyToknames = [...]string{
	"$end",
	"error",
	"$unk",
	"ABORT",
	"ACTION",
	"AND",
	"ASC",
	"AUTOINCREMENT",
	"CASCADE",
	"CHECK",
	"COLLATE",
	"CONFLICT",
	"CONSTRAINT",
	"CREATE",
	"DEFAULT",
	"DEFERRABLE",
	"DEFERRED",
	"DELETE",
	"DESC",
	"FAIL",
	"FOREIGN",
	"FROM",
	"GLOB",
	"IGNORE",
	"IN",
	"INDEX",
	"INITIALLY",
	"IS",
	"KEY",
	"LIKE",
	"MATCH",
	"NO",
	"NOT",
	"NULL",
	"ON",
	"OR",
	"PRIMARY",
	"REFERENCES",
	"REGEXP",
	"REPLACE",
	"RESTRICT",
	"ROLLBACK",
	"ROWID",
	"SELECT",
	"SET",
	"TABLE",
	"UNIQUE",
	"UPDATE",
	"WHERE",
	"WITHOUT",
	"tBare",
	"tLiteral",
	"tIdentifier",
	"tOperator",
	"tSignedNumber",
	"tFloat",
	"'-'",
	"'+'",
	"','",
	"'('",
	"')'",
}
var yyStatenames = [...]string{}

const yyEofCode = 1
const yyErrCode = 2
const yyInitialStackSize = 16

//line yacctab:1
var yyExca = [...]int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 87,
	60, 6,
	-2, 93,
	-1, 88,
	60, 7,
	-2, 94,
}

const yyPrivate = 57344

const yyLast = 221

var yyAct = [...]int{

	171, 83, 81, 50, 9, 127, 84, 10, 78, 68,
	79, 98, 148, 85, 112, 55, 19, 114, 113, 10,
	22, 140, 24, 112, 82, 10, 114, 113, 27, 33,
	33, 35, 10, 121, 146, 27, 155, 108, 30, 152,
	59, 87, 86, 88, 119, 70, 92, 90, 91, 69,
	89, 75, 115, 76, 112, 67, 105, 114, 113, 77,
	151, 124, 150, 146, 18, 147, 73, 74, 94, 101,
	70, 66, 71, 72, 54, 108, 96, 142, 102, 103,
	108, 107, 109, 106, 39, 51, 40, 70, 92, 90,
	91, 70, 116, 71, 72, 52, 42, 102, 103, 23,
	62, 17, 38, 120, 117, 118, 12, 10, 13, 169,
	128, 73, 74, 132, 129, 135, 136, 137, 139, 130,
	97, 134, 133, 10, 14, 16, 128, 143, 141, 28,
	28, 6, 11, 11, 36, 149, 11, 178, 63, 170,
	12, 12, 13, 13, 12, 173, 13, 58, 10, 165,
	63, 158, 47, 49, 159, 163, 177, 48, 154, 161,
	29, 5, 41, 56, 65, 166, 95, 26, 175, 167,
	93, 176, 64, 57, 60, 45, 44, 174, 32, 43,
	51, 172, 145, 20, 99, 168, 8, 164, 157, 46,
	126, 38, 160, 111, 123, 179, 100, 53, 37, 153,
	144, 125, 138, 80, 21, 131, 156, 34, 162, 31,
	122, 110, 61, 15, 25, 7, 104, 4, 3, 2,
	1,
}
var yyPact = [...]int{

	117, -1000, -1000, -1000, -1000, 93, 78, 42, -1000, -1000,
	-1000, -1000, -1000, -1000, 55, 157, -1000, 93, 55, 39,
	55, -1000, -1000, 90, 125, -21, -1000, 55, 55, 55,
	89, 25, 142, 35, 142, 14, 126, -1000, 55, 178,
	50, 142, -1000, 143, -1000, 130, -1000, 11, 15, 55,
	-1000, 55, 36, 142, -10, -1000, 141, 8, 137, -1000,
	126, -1000, 77, -1000, 177, -1000, -10, -1000, -1000, -1000,
	-1000, 36, 36, -1000, -1000, -1000, -4, 22, 21, -1000,
	182, -31, -1000, -8, -1000, -1000, -1000, -1000, -1000, -10,
	32, 32, -1000, -16, -10, -27, -1000, -1000, 186, -1000,
	-1000, 0, -1000, -1000, 174, 93, -1000, 36, -10, 64,
	177, 60, -10, -10, -10, -10, -40, -1000, -1000, -10,
	16, 93, -1000, -1000, -1000, 155, -1000, 4, -1000, -49,
	-1000, -1000, -10, -1000, -1000, -31, -31, -31, 1, -31,
	-1000, -22, 123, -25, -1000, 171, 93, -1000, -1000, -31,
	-1000, -10, -1000, -1000, 180, 47, 120, -1000, -1000, -31,
	145, -1000, -1000, 91, -1000, -1000, -1000, -1000, -1000, 136,
	136, -1000, 122, -1000, -1000, 190, -1000, -1000, -1000, -1000,
}
var yyPgo = [...]int{

	0, 220, 219, 218, 217, 1, 9, 6, 13, 4,
	186, 5, 216, 215, 214, 167, 8, 10, 178, 134,
	213, 212, 211, 11, 210, 96, 162, 15, 209, 3,
	0, 208, 206, 205, 203, 2, 202, 201, 200, 199,
}
var yyR1 = [...]int{

	0, 1, 1, 1, 6, 6, 5, 5, 7, 7,
	7, 8, 8, 8, 9, 9, 11, 11, 12, 10,
	13, 13, 25, 25, 25, 25, 25, 25, 25, 25,
	25, 25, 26, 26, 26, 27, 27, 27, 29, 19,
	19, 28, 28, 28, 24, 24, 14, 14, 15, 15,
	18, 18, 18, 18, 22, 22, 23, 23, 23, 21,
	21, 20, 20, 39, 39, 39, 39, 39, 39, 16,
	16, 34, 17, 30, 30, 30, 30, 30, 31, 31,
	32, 32, 37, 37, 38, 38, 33, 33, 35, 35,
	35, 35, 35, 35, 35, 35, 35, 35, 35, 36,
	36, 36, 2, 3, 4,
}
var yyR2 = [...]int{

	0, 1, 1, 1, 1, 1, 1, 1, 1, 2,
	2, 1, 2, 2, 1, 1, 1, 3, 3, 1,
	1, 3, 4, 1, 2, 1, 4, 2, 2, 2,
	2, 1, 0, 1, 2, 5, 5, 6, 6, 0,
	2, 0, 3, 4, 0, 1, 1, 3, 3, 3,
	0, 1, 4, 6, 0, 2, 0, 1, 1, 0,
	2, 0, 1, 0, 3, 3, 3, 3, 3, 1,
	3, 1, 3, 2, 2, 1, 1, 2, 3, 3,
	0, 2, 0, 1, 0, 2, 0, 2, 1, 4,
	1, 1, 1, 1, 1, 3, 3, 3, 3, 0,
	1, 3, 4, 8, 10,
}
var yyChk = [...]int{

	-1000, -1, -2, -3, -4, 44, 14, -13, -10, -9,
	-5, 43, 51, 53, 46, -20, 47, 59, 22, -5,
	26, -10, -5, 60, -5, -14, -15, -9, 40, 35,
	59, -28, -18, -5, -18, -5, -19, -15, 13, 59,
	61, -26, -25, 37, 34, 33, 47, 10, 15, 11,
	-29, 38, 60, -26, 60, -27, 37, 47, 21, -5,
	-19, -21, 50, -25, 29, 34, 60, -7, -6, 34,
	55, 57, 58, 51, 52, -5, -5, -7, -16, -17,
	-34, -35, 34, -5, -7, -8, 52, 51, 53, 60,
	57, 58, 56, 29, 60, 29, -27, 43, -23, 7,
	19, -35, -7, -7, -12, 60, 61, 59, 59, 61,
	-22, 11, 54, 58, 57, 60, -35, -8, -8, 60,
	-16, 60, -24, 8, 61, -37, 16, -11, -9, -7,
	-17, -33, 49, -23, -6, -35, -35, -35, -36, -35,
	61, -16, 61, -11, -38, 27, 59, 61, 61, -35,
	61, 59, 61, -39, 35, 61, -32, 17, -9, -35,
	12, -29, -31, 35, 42, 4, 20, 24, 40, 18,
	48, -30, 45, 9, 41, 32, -30, 34, 15, 5,
}
var yyDef = [...]int{

	0, -2, 1, 2, 3, 0, 61, 0, 20, 19,
	14, 15, 6, 7, 0, 0, 62, 0, 0, 0,
	0, 21, 102, 0, 0, 41, 46, 50, 50, 0,
	39, 0, 32, 51, 32, 0, 0, 47, 0, 39,
	59, 48, 33, 0, 23, 0, 25, 0, 0, 0,
	31, 0, 0, 49, 0, 42, 0, 0, 0, 40,
	0, 103, 0, 34, 56, 24, 0, 27, 28, 29,
	8, 0, 0, 4, 5, 30, 0, 0, 0, 69,
	54, 71, 88, 0, 90, 91, 92, -2, -2, 0,
	0, 0, 11, 0, 0, 0, 43, 60, 44, 57,
	58, 0, 9, 10, 82, 0, 52, 0, 0, 86,
	56, 0, 0, 0, 0, 99, 0, 12, 13, 0,
	0, 0, 22, 45, 26, 84, 83, 0, 16, 0,
	70, 104, 0, 72, 55, 95, 96, 97, 0, 100,
	98, 0, 63, 0, 80, 0, 0, 18, 53, 87,
	89, 0, 35, 36, 0, 0, 38, 85, 17, 101,
	0, 37, 81, 0, 64, 65, 66, 67, 68, 0,
	0, 78, 0, 75, 76, 0, 79, 73, 74, 77,
}
var yyTok1 = [...]int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	60, 61, 3, 58, 59, 57,
}
var yyTok2 = [...]int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
	32, 33, 34, 35, 36, 37, 38, 39, 40, 41,
	42, 43, 44, 45, 46, 47, 48, 49, 50, 51,
	52, 53, 54, 55, 56,
}
var yyTok3 = [...]int{
	0,
}

var yyErrorMessages = [...]struct {
	state int
	token int
	msg   string
}{}

//line yaccpar:1

/*	parser for yacc output	*/

var (
	yyDebug        = 0
	yyErrorVerbose = false
)

type yyLexer interface {
	Lex(lval *yySymType) int
	Error(s string)
}

type yyParser interface {
	Parse(yyLexer) int
	Lookahead() int
}

type yyParserImpl struct {
	lval  yySymType
	stack [yyInitialStackSize]yySymType
	char  int
}

func (p *yyParserImpl) Lookahead() int {
	return p.char
}

func yyNewParser() yyParser {
	return &yyParserImpl{}
}

const yyFlag = -1000

func yyTokname(c int) string {
	if c >= 1 && c-1 < len(yyToknames) {
		if yyToknames[c-1] != "" {
			return yyToknames[c-1]
		}
	}
	return __yyfmt__.Sprintf("tok-%v", c)
}

func yyStatname(s int) string {
	if s >= 0 && s < len(yyStatenames) {
		if yyStatenames[s] != "" {
			return yyStatenames[s]
		}
	}
	return __yyfmt__.Sprintf("state-%v", s)
}

func yyErrorMessage(state, lookAhead int) string {
	const TOKSTART = 4

	if !yyErrorVerbose {
		return "syntax error"
	}

	for _, e := range yyErrorMessages {
		if e.state == state && e.token == lookAhead {
			return "syntax error: " + e.msg
		}
	}

	res := "syntax error: unexpected " + yyTokname(lookAhead)

	// To match Bison, suggest at most four expected tokens.
	expected := make([]int, 0, 4)

	// Look for shiftable tokens.
	base := yyPact[state]
	for tok := TOKSTART; tok-1 < len(yyToknames); tok++ {
		if n := base + tok; n >= 0 && n < yyLast && yyChk[yyAct[n]] == tok {
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}
	}

	if yyDef[state] == -2 {
		i := 0
		for yyExca[i] != -1 || yyExca[i+1] != state {
			i += 2
		}

		// Look for tokens that we accept or reduce.
		for i += 2; yyExca[i] >= 0; i += 2 {
			tok := yyExca[i]
			if tok < TOKSTART || yyExca[i+1] == 0 {
				continue
			}
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}

		// If the default action is to accept or reduce, give up.
		if yyExca[i+1] != 0 {
			return res
		}
	}

	for i, tok := range expected {
		if i == 0 {
			res += ", expecting "
		} else {
			res += " or "
		}
		res += yyTokname(tok)
	}
	return res
}

func yylex1(lex yyLexer, lval *yySymType) (char, token int) {
	token = 0
	char = lex.Lex(lval)
	if char <= 0 {
		token = yyTok1[0]
		goto out
	}
	if char < len(yyTok1) {
		token = yyTok1[char]
		goto out
	}
	if char >= yyPrivate {
		if char < yyPrivate+len(yyTok2) {
			token = yyTok2[char-yyPrivate]
			goto out
		}
	}
	for i := 0; i < len(yyTok3); i += 2 {
		token = yyTok3[i+0]
		if token == char {
			token = yyTok3[i+1]
			goto out
		}
	}

out:
	if token == 0 {
		token = yyTok2[1] /* unknown char */
	}
	if yyDebug >= 3 {
		__yyfmt__.Printf("lex %s(%d)\n", yyTokname(token), uint(char))
	}
	return char, token
}

func yyParse(yylex yyLexer) int {
	return yyNewParser().Parse(yylex)
}

func (yyrcvr *yyParserImpl) Parse(yylex yyLexer) int {
	var yyn int
	var yyVAL yySymType
	var yyDollar []yySymType
	_ = yyDollar // silence set and not used
	yyS := yyrcvr.stack[:]

	Nerrs := 0   /* number of errors */
	Errflag := 0 /* error recovery flag */
	yystate := 0
	yyrcvr.char = -1
	yytoken := -1 // yyrcvr.char translated into internal numbering
	defer func() {
		// Make sure we report no lookahead when not parsing.
		yystate = -1
		yyrcvr.char = -1
		yytoken = -1
	}()
	yyp := -1
	goto yystack

ret0:
	return 0

ret1:
	return 1

yystack:
	/* put a state and value onto the stack */
	if yyDebug >= 4 {
		__yyfmt__.Printf("char %v in %v\n", yyTokname(yytoken), yyStatname(yystate))
	}

	yyp++
	if yyp >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyS[yyp] = yyVAL
	yyS[yyp].yys = yystate

yynewstate:
	yyn = yyPact[yystate]
	if yyn <= yyFlag {
		goto yydefault /* simple state */
	}
	if yyrcvr.char < 0 {
		yyrcvr.char, yytoken = yylex1(yylex, &yyrcvr.lval)
	}
	yyn += yytoken
	if yyn < 0 || yyn >= yyLast {
		goto yydefault
	}
	yyn = yyAct[yyn]
	if yyChk[yyn] == yytoken { /* valid shift */
		yyrcvr.char = -1
		yytoken = -1
		yyVAL = yyrcvr.lval
		yystate = yyn
		if Errflag > 0 {
			Errflag--
		}
		goto yystack
	}

yydefault:
	/* default state action */
	yyn = yyDef[yystate]
	if yyn == -2 {
		if yyrcvr.char < 0 {
			yyrcvr.char, yytoken = yylex1(yylex, &yyrcvr.lval)
		}

		/* look through exception table */
		xi := 0
		for {
			if yyExca[xi+0] == -1 && yyExca[xi+1] == yystate {
				break
			}
			xi += 2
		}
		for xi += 2; ; xi += 2 {
			yyn = yyExca[xi+0]
			if yyn < 0 || yyn == yytoken {
				break
			}
		}
		yyn = yyExca[xi+1]
		if yyn < 0 {
			goto ret0
		}
	}
	if yyn == 0 {
		/* error ... attempt to resume parsing */
		switch Errflag {
		case 0: /* brand new error */
			yylex.Error(yyErrorMessage(yystate, yytoken))
			Nerrs++
			if yyDebug >= 1 {
				__yyfmt__.Printf("%s", yyStatname(yystate))
				__yyfmt__.Printf(" saw %s\n", yyTokname(yytoken))
			}
			fallthrough

		case 1, 2: /* incompletely recovered error ... try again */
			Errflag = 3

			/* find a state where "error" is a legal shift action */
			for yyp >= 0 {
				yyn = yyPact[yyS[yyp].yys] + yyErrCode
				if yyn >= 0 && yyn < yyLast {
					yystate = yyAct[yyn] /* simulate a shift of "error" */
					if yyChk[yystate] == yyErrCode {
						goto yystack
					}
				}

				/* the current p has no shift on "error", pop stack */
				if yyDebug >= 2 {
					__yyfmt__.Printf("error recovery pops state %d\n", yyS[yyp].yys)
				}
				yyp--
			}
			/* there is no state on the stack with an error shift ... abort */
			goto ret1

		case 3: /* no shift yet; clobber input char */
			if yyDebug >= 2 {
				__yyfmt__.Printf("error recovery discards %s\n", yyTokname(yytoken))
			}
			if yytoken == yyEofCode {
				goto ret1
			}
			yyrcvr.char = -1
			yytoken = -1
			goto yynewstate /* try again in the same state */
		}
	}

	/* reduction by production yyn */
	if yyDebug >= 2 {
		__yyfmt__.Printf("reduce %v in:\n\t%v\n", yyn, yyStatname(yystate))
	}

	yynt := yyn
	yypt := yyp
	_ = yypt // guard against "declared and not used"

	yyp -= yyR2[yyn]
	// yyp is now the index of $0. Perform the default action. Iff the
	// reduced production is ??, $1 is possibly out of range.
	if yyp+1 >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyVAL = yyS[yyp+1]

	/* consult goto table to find next state */
	yyn = yyR1[yyn]
	yyg := yyPgo[yyn]
	yyj := yyg + yyS[yyp].yys + 1

	if yyj >= yyLast {
		yystate = yyAct[yyg]
	} else {
		yystate = yyAct[yyj]
		if yyChk[yystate] != -yyn {
			yystate = yyAct[yyg]
		}
	}
	// dummy call; replaced with literal code
	switch yynt {

	case 4:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:131
		{
			yyVAL.literal = yyDollar[1].identifier
		}
	case 5:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:134
		{
			yyVAL.literal = yyDollar[1].identifier
		}
	case 6:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:139
		{
			yyVAL.identifier = yyDollar[1].identifier
		}
	case 7:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:142
		{
			yyVAL.identifier = yyDollar[1].identifier
		}
	case 8:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:147
		{
			yyVAL.signedNumber = yyDollar[1].signedNumber
		}
	case 9:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:150
		{
			yyVAL.signedNumber = -yyDollar[2].signedNumber
		}
	case 10:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:153
		{
			yyVAL.signedNumber = yyDollar[2].signedNumber
		}
	case 11:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:158
		{
			yyVAL.float = yyDollar[1].float
		}
	case 12:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:161
		{
			yyVAL.float = -yyDollar[2].float
		}
	case 13:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:164
		{
			yyVAL.float = yyDollar[2].float
		}
	case 14:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:169
		{
			yyVAL.columnName = yyDollar[1].identifier
		}
	case 15:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:172
		{
			yyVAL.columnName = "ROWID"
		}
	case 16:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:177
		{
			yyVAL.columnNameList = []string{yyDollar[1].columnName}
		}
	case 17:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:180
		{
			yyVAL.columnNameList = append(yyDollar[1].columnNameList, yyDollar[3].columnName)
		}
	case 18:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:185
		{
			yyVAL.columnNameList = yyDollar[2].columnNameList
		}
	case 19:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:190
		{
			yyVAL.columnName = yyDollar[1].columnName
		}
	case 20:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:195
		{
			yyVAL.columnNameList = []string{yyDollar[1].columnName}
		}
	case 21:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:198
		{
			yyVAL.columnNameList = append(yyDollar[1].columnNameList, yyDollar[3].columnName)
		}
	case 22:
		yyDollar = yyS[yypt-4 : yypt+1]
//line parser.go.y:204
		{
			yyVAL.columnConstraint = ccPrimaryKey{yyDollar[3].sortOrder, yyDollar[4].bool}
		}
	case 23:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:207
		{
			yyVAL.columnConstraint = ccNull(true)
		}
	case 24:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:210
		{
			yyVAL.columnConstraint = ccNull(false)
		}
	case 25:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:213
		{
			yyVAL.columnConstraint = ccUnique(true)
		}
	case 26:
		yyDollar = yyS[yypt-4 : yypt+1]
//line parser.go.y:216
		{
			yyVAL.columnConstraint = ccCheck{expr: yyDollar[3].expr}
		}
	case 27:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:219
		{
			yyVAL.columnConstraint = ccDefault(yyDollar[2].signedNumber)
		}
	case 28:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:222
		{
			yyVAL.columnConstraint = ccDefault(yyDollar[2].literal)
		}
	case 29:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:225
		{
			yyVAL.columnConstraint = ccDefault(nil)
		}
	case 30:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:228
		{
			yyVAL.columnConstraint = ccCollate(yyDollar[2].identifier)
		}
	case 31:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:231
		{
			yyVAL.columnConstraint = ccReferences(yyDollar[1].foreignKeyClause)
		}
	case 32:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:236
		{
			yyVAL.columnConstraintList = nil
		}
	case 33:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:239
		{
			yyVAL.columnConstraintList = []columnConstraint{yyDollar[1].columnConstraint}
		}
	case 34:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:242
		{
			yyVAL.columnConstraintList = append(yyDollar[1].columnConstraintList, yyDollar[2].columnConstraint)
		}
	case 35:
		yyDollar = yyS[yypt-5 : yypt+1]
//line parser.go.y:247
		{
			yyVAL.tableConstraint = TablePrimaryKey{yyDollar[4].indexedColumnList}
		}
	case 36:
		yyDollar = yyS[yypt-5 : yypt+1]
//line parser.go.y:250
		{
			yyVAL.tableConstraint = TableUnique{
				IndexedColumns: yyDollar[3].indexedColumnList,
			}
		}
	case 37:
		yyDollar = yyS[yypt-6 : yypt+1]
//line parser.go.y:255
		{
			yyVAL.tableConstraint = TableForeignKey{
				Columns: yyDollar[4].columnNameList,
				Clause:  yyDollar[6].foreignKeyClause,
			}
		}
	case 38:
		yyDollar = yyS[yypt-6 : yypt+1]
//line parser.go.y:263
		{
			yyVAL.foreignKeyClause = ForeignKeyClause{
				ForeignTable:      yyDollar[2].identifier,
				ForeignColumns:    yyDollar[3].columnNameList,
				Deferrable:        yyDollar[4].bool,
				InitiallyDeferred: yyDollar[5].bool,
				Triggers:          yyDollar[6].triggerList,
			}
		}
	case 39:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:274
		{
		}
	case 40:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:275
		{
		}
	case 41:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:279
		{
		}
	case 42:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:280
		{
			yyVAL.tableConstraintList = []TableConstraint{yyDollar[3].tableConstraint}
		}
	case 43:
		yyDollar = yyS[yypt-4 : yypt+1]
//line parser.go.y:283
		{
			yyVAL.tableConstraintList = append(yyDollar[1].tableConstraintList, yyDollar[4].tableConstraint)
		}
	case 44:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:289
		{
		}
	case 45:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:290
		{
			yyVAL.bool = true
		}
	case 46:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:295
		{
			yyVAL.columnDefList = []ColumnDef{yyDollar[1].columnDef}
		}
	case 47:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:298
		{
			yyVAL.columnDefList = append(yyDollar[1].columnDefList, yyDollar[3].columnDef)
		}
	case 48:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:303
		{
			yyVAL.columnDef = makeColumnDef(yyDollar[1].columnName, yyDollar[2].name, yyDollar[3].columnConstraintList)
		}
	case 49:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:306
		{
			yyVAL.columnDef = makeColumnDef("REPLACE", yyDollar[2].name, yyDollar[3].columnConstraintList)
		}
	case 50:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:311
		{
			yyVAL.name = ""
		}
	case 51:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:314
		{
			yyVAL.name = yyDollar[1].identifier
		}
	case 52:
		yyDollar = yyS[yypt-4 : yypt+1]
//line parser.go.y:317
		{
			yyVAL.name = yyDollar[1].identifier
		}
	case 53:
		yyDollar = yyS[yypt-6 : yypt+1]
//line parser.go.y:320
		{
			yyVAL.name = yyDollar[1].identifier
		}
	case 54:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:325
		{
		}
	case 55:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:326
		{
			yyVAL.collate = yyDollar[2].literal
		}
	case 56:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:331
		{
			yyVAL.sortOrder = Asc
		}
	case 57:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:334
		{
			yyVAL.sortOrder = Asc
		}
	case 58:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:337
		{
			yyVAL.sortOrder = Desc
		}
	case 59:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:342
		{
			yyVAL.withoutRowid = false
		}
	case 60:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:345
		{
			yyVAL.withoutRowid = true
		}
	case 61:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:350
		{
			yyVAL.unique = false
		}
	case 62:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:353
		{
			yyVAL.unique = true
		}
	case 63:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:358
		{
		}
	case 64:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:359
		{
		}
	case 65:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:361
		{
		}
	case 66:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:363
		{
		}
	case 67:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:365
		{
		}
	case 68:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:367
		{
		}
	case 69:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:371
		{
			yyVAL.indexedColumnList = []IndexedColumn{yyDollar[1].indexedColumn}
		}
	case 70:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:374
		{
			yyVAL.indexedColumnList = append(yyDollar[1].indexedColumnList, yyDollar[3].indexedColumn)
		}
	case 71:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:379
		{
			yyVAL.expr = yyDollar[1].expr
		}
	case 72:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:384
		{
			yyVAL.indexedColumn = newIndexColumn(yyDollar[1].expr, yyDollar[2].collate, yyDollar[3].sortOrder)
		}
	case 73:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:389
		{
			yyVAL.triggerAction = ActionSetNull
		}
	case 74:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:392
		{
			yyVAL.triggerAction = ActionSetDefault
		}
	case 75:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:395
		{
			yyVAL.triggerAction = ActionCascade
		}
	case 76:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:398
		{
			yyVAL.triggerAction = ActionRestrict
		}
	case 77:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:401
		{
			yyVAL.triggerAction = ActionNoAction
		}
	case 78:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:406
		{
			yyVAL.trigger = TriggerOnDelete(yyDollar[3].triggerAction)
		}
	case 79:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:409
		{
			yyVAL.trigger = TriggerOnUpdate(yyDollar[3].triggerAction)
		}
	case 80:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:414
		{
		}
	case 81:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:415
		{
			yyVAL.triggerList = append(yyDollar[1].triggerList, yyDollar[2].trigger)
		}
	case 82:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:420
		{
			yyVAL.bool = false
		}
	case 83:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:423
		{
			yyVAL.bool = true
		}
	case 84:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:428
		{
			yyVAL.bool = false
		}
	case 85:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:431
		{
			yyVAL.bool = true
		}
	case 86:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:436
		{
		}
	case 87:
		yyDollar = yyS[yypt-2 : yypt+1]
//line parser.go.y:437
		{
			yyVAL.where = yyDollar[2].expr
		}
	case 88:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:442
		{
			yyVAL.expr = nil
		}
	case 89:
		yyDollar = yyS[yypt-4 : yypt+1]
//line parser.go.y:445
		{
			yyVAL.expr = ExFunction{yyDollar[1].identifier, yyDollar[3].exprList}
		}
	case 90:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:448
		{
			yyVAL.expr = yyDollar[1].signedNumber
		}
	case 91:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:451
		{
			yyVAL.expr = yyDollar[1].float
		}
	case 92:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:454
		{
			yyVAL.expr = yyDollar[1].identifier
		}
	case 93:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:457
		{
			yyVAL.expr = ExColumn(yyDollar[1].identifier)
		}
	case 94:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:460
		{
			yyVAL.expr = ExColumn(yyDollar[1].identifier)
		}
	case 95:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:463
		{
			yyVAL.expr = ExBinaryOp{yyDollar[2].identifier, yyDollar[1].expr, yyDollar[3].expr}
		}
	case 96:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:466
		{
			yyVAL.expr = ExBinaryOp{"+", yyDollar[1].expr, yyDollar[3].expr}
		}
	case 97:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:469
		{
			yyVAL.expr = ExBinaryOp{"-", yyDollar[1].expr, yyDollar[3].expr}
		}
	case 98:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:472
		{
			yyVAL.expr = yyDollar[2].expr
		}
	case 99:
		yyDollar = yyS[yypt-0 : yypt+1]
//line parser.go.y:477
		{
			yyVAL.exprList = nil
		}
	case 100:
		yyDollar = yyS[yypt-1 : yypt+1]
//line parser.go.y:480
		{
			yyVAL.exprList = []Expression{yyDollar[1].expr}
		}
	case 101:
		yyDollar = yyS[yypt-3 : yypt+1]
//line parser.go.y:483
		{
			yyVAL.exprList = append(yyDollar[1].exprList, yyDollar[3].expr)
		}
	case 102:
		yyDollar = yyS[yypt-4 : yypt+1]
//line parser.go.y:488
		{
			yylex.(*lexer).result = SelectStmt{Columns: yyDollar[2].columnNameList, Table: yyDollar[4].identifier}
		}
	case 103:
		yyDollar = yyS[yypt-8 : yypt+1]
//line parser.go.y:493
		{
			yylex.(*lexer).result = CreateTableStmt{
				Table:        yyDollar[3].identifier,
				Columns:      yyDollar[5].columnDefList,
				Constraints:  yyDollar[6].tableConstraintList,
				WithoutRowid: yyDollar[8].withoutRowid,
			}
		}
	case 104:
		yyDollar = yyS[yypt-10 : yypt+1]
//line parser.go.y:503
		{
			yylex.(*lexer).result = CreateIndexStmt{
				Index:          yyDollar[4].identifier,
				Table:          yyDollar[6].identifier,
				Unique:         yyDollar[2].unique,
				IndexedColumns: yyDollar[8].indexedColumnList,
				Where:          yyDollar[10].where,
			}
		}
	}
	goto yystack /* stack new state and value */
}
