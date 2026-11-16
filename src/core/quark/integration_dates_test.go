package main

import "testing"

// Dates v0.1 (specs/dates_v0_1_spec.md): the date and period types, calendar
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
	})
}
