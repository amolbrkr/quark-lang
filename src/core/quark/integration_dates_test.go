package main

import "testing"

// Dates v0.1 (specs/implemented/dates_v0_1_spec.md): the date and period types, calendar
// arithmetic, and date columns in tables and CSV.
func TestDates(t *testing.T) {
	runTableCases(t, []tableCase{
		{
			name: "arithmetic",
			source: `incl 'std/time' as time
d: date = time::ymd('2022-01-30')
print(d + time::months(3))
print(d + time::days(7))
print(d + 7)
print(time::date(2026, 1, 1) - d)
print(time::months(1) + time::days(2))
print(3 * time::months(1))
print(time::ymd('2022-01-31') + time::months(1))
print(time::ymd('2024-01-31') + time::months(1))
print(time::ymd('2022-03-31') - time::months(1))
print(type(d))
print(time::to_days(time::from_days(0)))
print(time::year(d))
print(time::month(d))
print(time::day(d))
`,
			want: "2022-04-30\n2022-02-06\n2022-02-06\n1432\n1 month 2 days\n3 months\n2022-02-28\n2024-02-29\n2022-02-28\ndate\n0\n2022\n1\n30",
		},
		{
			name: "query_and_csv",
			files: map[string]string{
				"events.csv": "id,created_at\n1,2022-01-01\n2,2022-06-15\n3,2023-01-02\n4,\n",
			},
			source: `incl 'std/time' as time
incl 'std/csv' as csv
struct Event:
    id: int
    created_at: date
cutoff: date = time::ymd('2022-01-30')
events: tbl[Event] = unwrap(csv::read_file('events.csv'))
recent = events | e -> filter(e.created_at > cutoff) | sort(e.created_at)
for d in recent.created_at:
    print(d)
later = events | e -> filter(e.created_at != null) | mutate(shifted = e.created_at + time::months(1))
for d in later.shifted:
    print(d)
struct Day:
    day: date
typed: tbl[Day] = unwrap(csv::parse_text('day\n2024-1-2\n2024-02-03\n'))
for d in typed.day:
    print(d)
`,
			want: "2022-06-15\n2023-01-02\n2022-02-01\n2022-07-15\n2023-02-02\n2024-01-02\n2024-02-03",
		},
		{name: "bad_day", source: "incl 'std/time' as time\nprint(time::date(2025, 2, 29))\n", err: "not a real calendar day"},
		{name: "bad_ymd", source: "incl 'std/time' as time\nprint(time::ymd('nope'))\n", err: "cannot parse 'nope'"},
		{name: "to_int", source: "incl 'std/time' as time\nprint(to_int(time::date(2020, 1, 1)))\n", err: "to_int() cannot convert a date"},
		{name: "vec_literal", source: "incl 'std/time' as time\nv = vec [time::date(2020, 1, 1)]\n", err: "vec literals can't hold date"},
		{name: "add_dates", source: "incl 'std/time' as time\nprint(time::date(2020, 1, 1) + time::date(2020, 1, 2))\n", err: "cannot add two dates"},
		{name: "period_column", source: "incl 'std/time' as time\nstruct S:\n    p: period\nfn f(t: tbl[S]) -> 1\n", err: "field 'p' is period"},
		{name: "csv_bad_date", files: map[string]string{"e.csv": "created_at\n01/30/2022\n"}, source: "incl 'std/csv' as csv\nstruct E:\n    created_at: date\nx: tbl[E] = unwrap(csv::read_file('e.csv'))\n", err: "is not a date"},
		{name: "null_date", source: "incl 'std/time' as time\nd: date = null\n", err: "cannot assign value of type 'null' to 'date'"},
		{
			// Every vec and list operation on dates that compiles also runs
			// (D9): to_list keeps the element type, compares, min/max, fillna,
			// sort, printing through std/fmt.
			name: "date_vecs_and_lists",
			source: `incl 'std/time' as time
incl 'std/csv' as csv
incl 'std/fmt' as fmt
struct E:
    d: date
    n: int
t: tbl[E] = unwrap(csv::parse_text('d,n\n2022-01-05,1\n,2\n2021-03-01,3\n'))
d0 = time::ymd('2022-01-05')
print(t.d == d0)
print(t.d < d0)
print(d0 >= t.d)
print(min(t.d))
print(max(t.d))
print(t.d.fillna(d0))
xs = t.d.fillna(d0).to_list()
print(xs)
ys = list [time::ymd('2023-01-01'), time::ymd('2020-05-05'), d0]
ys.sort()
print(ys)
print(fmt::table(t))
`,
			want: "[true, null, false]\n[false, null, true]\n[true, null, true]\n2021-03-01\n2022-01-05\n[2022-01-05, 2022-01-05, 2021-03-01]\n[2022-01-05, 2022-01-05, 2021-03-01]\n[2020-05-05, 2022-01-05, 2023-01-01]\n" +
				"+---+------------+---+\n| # | d          | n |\n+---+------------+---+\n| 0 | 2022-01-05 | 1 |\n| 1 | NA         | 2 |\n| 2 | 2021-03-01 | 3 |\n+---+------------+---+\n[3 rows x 2 cols]",
		},
		{
			// A list made by to_list() of a vec[date] is a list[date]: a value of
			// another type pushed through an untyped function is an error.
			name:   "date_to_list_elem_type",
			source: "incl 'std/time' as time\nincl 'std/csv' as csv\nstruct E:\n    d: date\nt: tbl[E] = unwrap(csv::parse_text('d\\n2022-01-05\\n'))\nxs = t.d.to_list()\nfn add(l, v) -> l.push(v)\nadd(xs, 5)\n",
			err:    "push()",
		},
		{name: "mean_of_dates", source: "incl 'std/time' as time\nincl 'std/csv' as csv\nstruct E:\n    d: date\nt: tbl[E] = unwrap(csv::parse_text('d\\n2022-01-05\\n'))\nprint(mean(t.d))\n", err: "dates can't be averaged"},
		{name: "compare_date_vec_with_int", source: "incl 'std/time' as time\nincl 'std/csv' as csv\nstruct E:\n    d: date\nt: tbl[E] = unwrap(csv::parse_text('d\\n2022-01-05\\n'))\nprint(t.d < 3)\n", err: "dates compare with dates"},
		{name: "period_scale_overflow", source: "incl 'std/time' as time\nprint(time::years(4) * 4611686018427387904)\n", err: "period is too large"},
		{name: "date_add_overflow", source: "incl 'std/time' as time\nprint(time::ymd('2022-01-05') + 9223372036854775807)\n", err: "outside the range of dates"},
		{name: "date_years_overflow", source: "incl 'std/time' as time\nprint(time::date(0, 1, 1) + time::years(-2147483648))\n", err: "outside the range of dates"},
		{name: "two_digit_year", source: "incl 'std/time' as time\nprint(time::ymd('25-12-22'))\n", err: "cannot parse '25-12-22'"},
		{name: "csv_two_digit_year", files: map[string]string{"e.csv": "d\n25-12-22\n"}, source: "incl 'std/csv' as csv\nstruct E:\n    d: date\nx: tbl[E] = unwrap(csv::read_file('e.csv'))\n", err: "is not a date"},
		{
			// A column of two-digit-year text isn't inferred as dates; years
			// before 1000, negative years and years past 9999 print with four
			// or more digits and read back.
			name: "year_digits",
			source: `incl 'std/time' as time
incl 'std/csv' as csv
t = unwrap(csv::parse_text('a\n25-12-22\n01-02-03\n'))
print(t)
for s in list ['0044-03-15', '-0044-03-15', '12345-01-01', '-5877641-06-23']:
    d = time::ymd(s)
    print(d)
    print(time::ymd(to_str(d)) == d)
`,
			want: "tbl: 2 rows\na\nstr\n25-12-22\n01-02-03\n0044-03-15\ntrue\n-0044-03-15\ntrue\n12345-01-01\ntrue\n-5877641-06-23\ntrue",
		},
		{name: "abs_of_date_any", source: "incl 'std/time' as time\nfn id(v) -> v\nprint(abs(id(time::ymd('2022-01-05'))))\n", err: "got date"},
	})
}
