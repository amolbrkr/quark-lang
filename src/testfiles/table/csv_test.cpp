// CSV / TSV reading and writing (tables v0.1). argv[1] is a scratch directory.
#include "check.hpp"

#include <cmath>
#include <cstring>

using namespace quark;
using std::nullopt;

static QTable* parse(const std::string& text, const std::vector<csv::Column>* sch, std::string* err,
                     csv::Options opt = csv::Options(), char sep = ',') {
    const QStructDef* def = nullptr;
    if (sch) {
        std::vector<const char*> names;
        for (const csv::Column& c : *sch) names.push_back(q_strdup(c.name.c_str()));
        def = schema(names);
    }
    err->clear();
    return csv::parse_table(text.data(), text.size(), "data.csv", sep, opt, def, sch, err);
}

static bool fails_with(const std::string& text, const std::vector<csv::Column>* sch, const std::string& want,
                       csv::Options opt = csv::Options()) {
    std::string err;
    QTable* t = parse(text, sch, &err, opt);
    if (t != nullptr || err.find(want) == std::string::npos) {
        std::fprintf(stderr, "  wanted error containing: %s\n  got: %s\n", want.c_str(), t ? "(a table)" : err.c_str());
        return false;
    }
    return true;
}

static const std::vector<csv::Column> employee = {
    {"id", QVector::Type::I64},
    {"name", QVector::Type::STR},
    {"salary", QVector::Type::F64},
    {"active", QVector::Type::BOOL},
};

static void test_messy_typed() {
    // A BOM, CRLF line endings, a column the schema doesn't use (dept), the
    // schema's columns in another order than the file's, quoted separators,
    // an embedded line break, "" escapes, spaces around values and quotes,
    // NA / empty / quoted-empty values, and blank lines.
    const std::string text =
        "\xEF\xBB\xBF" "name,dept,id,salary,active\r\n"
        "\"Smith, Ann\",eng, 1 ,100000.5,TRUE\r\n"
        "\r\n"
        "  \"Bo \"\"The Boss\"\" Li\"  ,ops,2,NA,false\r\n"
        "\"multi\nline\",eng,3,  7e4 ,T\r\n"
        "   \r\n"
        ",ops,4,\"\",\r\n"
        "Zoë,hr,-5,-0.25,F\r\n";
    std::string err;
    QTable* t = parse(text, &employee, &err);
    EXPECT("messy parses", t != nullptr);
    if (!t) {
        std::fprintf(stderr, "  %s\n", err.c_str());
        return;
    }
    EXPECT("messy rows", t->nrows == 5 && t->ncols == 4 && well_formed(t));
    EXPECT("messy ids", ints(col(t, 0)) == std::vector<Opt>{1, 2, 3, 4, -5});
    EXPECT("messy names", strs(col(t, 1)) == std::vector<OptS>{"Smith, Ann", "Bo \"The Boss\" Li", "multi\nline", nullopt, "Zoë"});
    EXPECT("messy salaries", floats(col(t, 2)) == std::vector<OptF>{100000.5, nullopt, 70000.0, nullopt, -0.25});
    EXPECT("messy bools", bools(col(t, 3)) == std::vector<OptB>{true, false, true, nullopt, false});
    EXPECT("column names follow the schema", std::strcmp(t->def->field_names[0], "id") == 0);

    // Old Mac line endings (\r only), no trailing newline.
    QTable* cr = parse("id,name,salary,active\r1,a,1.5,true\r2,b,2,false", &employee, &err);
    EXPECT("CR line endings", cr && cr->nrows == 2 && ints(col(cr, 0)) == std::vector<Opt>{1, 2});

    // A header only is an empty table with the schema's columns.
    QTable* hdr = parse("id,name,salary,active\n", &employee, &err);
    EXPECT("header only", hdr && hdr->nrows == 0 && hdr->ncols == 4 && well_formed(hdr));

    // A quote inside an unquoted field is a literal character.
    QTable* q = parse("id,name,salary,active\n1,5\" screen,1,true\n", &employee, &err);
    EXPECT("inner quote is literal", q && strs(col(q, 1)) == std::vector<OptS>{"5\" screen"});

    // Trimming can be turned off; NA strings can be changed.
    csv::Options raw;
    raw.trim = false;
    raw.na = {"-"};
    std::vector<csv::Column> one = {{"name", QVector::Type::STR}};
    QTable* r = parse("name\n  padded \n-\nNA\n", &one, &err, raw);
    EXPECT("no trim, custom NA", r && strs(col(r, 0)) == std::vector<OptS>{"  padded ", nullopt, "NA"});
}

static void test_errors() {
    const std::string head = "id,name,salary,active\n";
    EXPECT("bad int", fails_with(head + "1,a,1,true\n2.5,b,1,true\n", &employee,
                                 "data.csv:3: column 'id': '2.5' is not an int"));
    EXPECT("bad float", fails_with(head + "1,a,12k,true\n", &employee, "data.csv:2: column 'salary': '12k' is not a float"));
    EXPECT("bad bool", fails_with(head + "1,a,1,yes\n", &employee, "column 'active': 'yes' is not a bool"));
    EXPECT("int overflow", fails_with(head + "99999999999999999999,a,1,true\n", &employee,
                                      "'99999999999999999999' is not an int"));
    EXPECT("hex is not a float", fails_with(head + "1,a,0x10,true\n", &employee, "'0x10' is not a float"));
    EXPECT("float overflow", fails_with(head + "1,a,1e999,true\n", &employee, "'1e999' is not a float"));
    EXPECT("too few fields", fails_with(head + "1,a,1\n", &employee, "data.csv:2: expected 4 fields, found 3"));
    EXPECT("too many fields", fails_with(head + "1,a,1,true,x\n", &employee, "expected 4 fields, found 5"));
    EXPECT("problems are counted", fails_with(head + "x,a,1,true\n2,b,y,true\n3,c\n", &employee,
                                              "data.csv:2: column 'id': 'x' is not an int (and 2 more problems)"));
    EXPECT("line numbers count quoted line breaks",
           fails_with(head + "1,\"a\nb\nc\",1,true\nz,d,1,true\n", &employee, "data.csv:5: column 'id': 'z' is not an int"));
    EXPECT("missing column", fails_with("id,name,pay,active\n", &employee,
                                        "no column named 'salary' in the header (columns: id, name, pay, active)"));
    EXPECT("duplicate column", fails_with("id,name,salary,active,id\n", &employee,
                                          "the header has 2 columns named 'id'"));
    EXPECT("unterminated quote", fails_with(head + "1,\"open,1,true\n2,b,1,true\n", &employee,
                                            "data.csv:2: a quoted field is never closed"));
    EXPECT("text after a closing quote", fails_with(head + "1,\"a\"b,1,true\n", &employee,
                                                    "data.csv:2: unexpected text after a closing quote"));
    EXPECT("empty file", fails_with("", &employee, "the file is empty (expected a header line)"));
    EXPECT("untyped empty header name", fails_with("a,,c\n1,2,3\n", nullptr, "column 2 has no name in the header"));
    EXPECT("untyped duplicate header", fails_with("a,b,a\n", nullptr, "the header has two columns named 'a'"));

    std::string err;
    csv::Options opt;
    QTable* missing = csv::read_table("/nonexistent/dir/x.csv", opt, nullptr, nullptr, &err);
    EXPECT("missing file", missing == nullptr && err.find("cannot open '/nonexistent/dir/x.csv'") == 0);
}

static void test_inference() {
    std::string err;
    QTable* t = parse("i,f,b,s,empty,mixed\n"
                      "1,1,true,x,,1\n"
                      "-2,2.5,F,y,NA,a\n"
                      "NA,1e3,TRUE,,,2\n",
                      nullptr, &err);
    EXPECT("infer parses", t != nullptr);
    if (!t) {
        std::fprintf(stderr, "  %s\n", err.c_str());
        return;
    }
    EXPECT("infer int", col(t, 0)->type == QVector::Type::I64 && ints(col(t, 0)) == std::vector<Opt>{1, -2, nullopt});
    EXPECT("infer float (ints and floats)", col(t, 1)->type == QVector::Type::F64 &&
                                                floats(col(t, 1)) == std::vector<OptF>{1.0, 2.5, 1000.0});
    EXPECT("infer bool", col(t, 2)->type == QVector::Type::BOOL);
    EXPECT("infer str", col(t, 3)->type == QVector::Type::STR && strs(col(t, 3)) == std::vector<OptS>{"x", "y", nullopt});
    EXPECT("all missing is str", col(t, 4)->type == QVector::Type::STR && col(t, 4)->null_count == 3);
    EXPECT("mixed is str", col(t, 5)->type == QVector::Type::STR && strs(col(t, 5)) == std::vector<OptS>{"1", "a", "2"});
    EXPECT("infer names", std::strcmp(t->def->field_names[1], "f") == 0 && t->def->field_count == 6);

    // The whole file is scanned: a float on the last row makes the column float.
    std::string many = "x\n";
    for (int i = 0; i < 5000; i++) many += std::to_string(i) + "\n";
    many += "0.5\n";
    QTable* late = parse(many, nullptr, &err);
    EXPECT("inference sees every row", late && col(late, 0)->type == QVector::Type::F64 && late->nrows == 5001);

    // No header: columns are x1, x2, ...; a schema maps by position.
    csv::Options nohdr;
    nohdr.header = false;
    QTable* nh = parse("1,a\n2,b\n", nullptr, &err, nohdr);
    EXPECT("no header inferred", nh && nh->nrows == 2 && std::strcmp(nh->def->field_names[1], "x2") == 0);
    std::vector<csv::Column> pos = {{"n", QVector::Type::I64}, {"s", QVector::Type::STR}};
    QTable* np = parse("1,a\n2,b\n", &pos, &err, nohdr);
    EXPECT("no header typed", np && ints(col(np, 0)) == std::vector<Opt>{1, 2});
    EXPECT("no header width mismatch", fails_with("1,a,x\n", &pos, "the schema has 2 columns but the file has 3", nohdr));
}

static void test_write(const std::string& dir) {
    QTable* t = table({"id", "name", "score", "ok"},
                      {i64_col({1, nullopt, -3}), str_col({"plain", "has, comma", " \"quoted\"\nline"}),
                       f64_col({0.1, nullopt, 1e21}), bool_col({true, nullopt, false})});
    const std::string text = csv::format_table(t, ',');
    EXPECT("format", text ==
                         "id,name,score,ok\n"
                         "1,plain,0.1,true\n"
                         ",\"has, comma\",,\n"
                         "-3,\" \"\"quoted\"\"\nline\",1e+21,false\n");

    // Round trip through a .tsv file (tab chosen from the extension).
    std::string err;
    csv::Options opt;
    const std::string path = dir + "/out.tsv";
    EXPECT("write tsv", csv::write_table(t, path.c_str(), opt, &err));
    std::vector<csv::Column> sch = {{"id", QVector::Type::I64}, {"name", QVector::Type::STR},
                                    {"score", QVector::Type::F64}, {"ok", QVector::Type::BOOL}};
    std::vector<const char*> names = {"id", "name", "score", "ok"};
    QTable* back = csv::read_table(path.c_str(), opt, schema(names), &sch, &err);
    EXPECT("read tsv back", back != nullptr);
    if (back) {
        EXPECT("round trip ints", ints(col(back, 0)) == ints(col(t, 0)));
        EXPECT("round trip strs", strs(col(back, 1)) == strs(col(t, 1)));
        EXPECT("round trip floats", floats(col(back, 2)) == floats(col(t, 2)));
        EXPECT("round trip bools", bools(col(back, 3)) == bools(col(t, 3)));
    } else {
        std::fprintf(stderr, "  %s\n", err.c_str());
    }
    std::string raw;
    EXPECT("tsv uses tabs", csv::read_file(path.c_str(), &raw, &err) && raw.rfind("id\tname\tscore\tok\n", 0) == 0);
    EXPECT("unwritable path", !csv::write_table(t, "/nonexistent/dir/x.csv", opt, &err) &&
                                  err.find("cannot open '/nonexistent/dir/x.csv' for writing") == 0);
}

static void test_scale() {
    // 500k rows x 6 columns (~30 MB): parse typed and untyped.
    std::string text = "id,name,dept,salary,start,active\n";
    text.reserve(32u << 20);
    char line[160];
    for (int i = 0; i < 500000; i++) {
        std::snprintf(line, sizeof line, "%d,\"Employee %d\",dept_%d,%d.%02d,2020-01-%02d,%s\n", i, i, i % 37,
                      40000 + (i * 7919) % 90000, i % 100, 1 + i % 28, (i % 3) ? "true" : "false");
        text += line;
    }
    std::vector<csv::Column> sch = {{"id", QVector::Type::I64}, {"name", QVector::Type::STR},
                                    {"salary", QVector::Type::F64}, {"active", QVector::Type::BOOL}};
    std::string err;
    Timer tt;
    QTable* t = parse(text, &sch, &err);
    const double typed_ms = tt.ms();
    EXPECT("scale typed", t && t->nrows == 500000 && well_formed(t));
    Timer ti;
    QTable* u = parse(text, nullptr, &err);
    const double untyped_ms = ti.ms();
    EXPECT("scale untyped", u && u->nrows == 500000 && col(u, 3)->type == QVector::Type::F64);
    std::printf("scale (500k rows, %.1f MB): typed read %.1f ms, inferred read %.1f ms\n",
                static_cast<double>(text.size()) / (1 << 20), typed_ms, untyped_ms);
}

int main(int argc, char** argv) {
    q_gc_init();
    const std::string dir = argc > 1 ? argv[1] : ".";
    test_messy_typed();
    test_errors();
    test_inference();
    test_write(dir);
    test_scale();
    return finish("table csv");
}
