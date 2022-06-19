# go-mysql-query-digest
Alternative to pt-query-digest in Golang

## Install

```sh
go install github.com/matsuu/go-mysql-query-digest@latest
```

## Usage

```sh
go-mysql-query-digest /path/to/mysql-slow.log
```

From STDIN:

```sh
cat /path/to/mysql-slow.log | go-mysql-query-digest
```

## Example

```
# Current date: Sun Jun 19 14:20:45 2022
# Hostname: m1macmini
# Files: /dev/stdin
# Overall: 768.80k total,  140.00 unique
# Attribute          total     min     max     avg     95%  median
# ============     ======= ======= ======= ======= ======= =======
# Exec time            95s       0      1s   123us    74us       0
# Lock time             7s       0   176ms     9us     6us       0
# Rows sent          1.69M       0  726.00    2.00    1.00       0
# Rows examine       2.56M       0   2.85k    3.00    1.00       0
# Rows affecte       1.23M       0  10.00k    1.00       0       0
# Bytes sent       300.08M       0   1.54M  409.00  841.00   11.00

# Profile
# Rank Query ID           Response time Calls  R/Call
# ==== ================== ============= ====== ======
#    1 0x21BCE747A4BE4A5C 30.2903 32.0% 123317 0.0002 SELECT isu
#    2 0x65D8BF6FCEA6F09F 24.4051 25.8%    126 0.1937 
#    3 0x3B08AB3679C9B193 19.4175 20.5%  25315 0.0008 SELECT isu_condition
#    4 0xD64A0900C3B8E744  4.3115  4.6%  41173 0.0001 SELECT user
#    5 0xCB1D5E69D512FD6E  4.0138  4.2%   2055 0.0020 SELECT isu_condition
#    6 0x23D19AC409FD97AF  3.5381  3.7%  18588 0.0002 SELECT isu_condition
#    7 0x9823A9B9ECA1A419  1.4828  1.6%   9218 0.0002 SELECT isu_condition
#    8 0x89EDAF00580A18D0  1.2887  1.4%    930 0.0014 SELECT isu_condition
#    9 0x17A1206082D32E11  1.1708  1.2%  27285 0.0000 SELECT isu
#   10 0x813031B8BBC3B329  0.9276  1.0%   6010 0.0002 COMMIT 
#   11 0xB711A299ADC32792  0.7797  0.8%    757 0.0010 
#   12 0x7043CE432C9FB05D  0.4406  0.5%    932 0.0005 SELECT isu_condition
#   13 0xA6C4FB46B2683CF0  0.4299  0.5%   3716 0.0001 SELECT isu
#   14 0xC76AB4AC0352B9E7  0.4285  0.5%    763 0.0006 INSERT user
#   15 0x1C267452BA9BBDB4  0.3128  0.3%   3681 0.0001 SELECT isu
#   16 0x609B7881A3741B33  0.2188  0.2%    618 0.0004 UPDATE isu_condition
#   17 0xFA4AECBA98E246CC  0.1916  0.2%   3415 0.0001 SELECT isu
#   18 0x93C643E21671646B  0.1563  0.2%    242 0.0006 INSERT isu
#   19 0x41D1CAD61AA0BD68  0.1477  0.2%   1675 0.0001 SELECT isu
#   20 0xACDFA234C0D2004A  0.0877  0.1%     67 0.0013 SELECT isu

# Query 1: ID 0x21BCE747A4BE4A5C
# Attribute    pct   total     min     max     avg     95%  median
# ============ === ======= ======= ======= ======= ======= =======
# Count         16  123317
# Exec time     32     30s     8us   399ms   246us    98us    14us
# Lock time     27      2s     1us    20ms    15us    16us     4us
# Rows sent      7 123.25k       0    1.00       0    1.00    1.00
# Rows examine   5 123.25k       0    1.00       0    1.00    1.00
# Rows affecte   0       0       0       0       0       0       0
# Bytes sent     5  14.93M   84.00  127.00  126.00  127.00  127.00
# String:
# Databases    isucondition
# 
# EXPLAIN /*!50100 PARTITIONS*/
SELECT jia_isu_uuid FROM `isu` WHERE `jia_isu_uuid` = '8bdae9a6-798a-4634-93fb-bccc67f37504'\G

...
```

## References

* [percona/go-mysql](https://github.com/percona/go-mysql) Go packages for MySQL
* [pingcap/parser](https://github.com/pingcap/parser) A MySQL Compatible SQL Parser
* [percona/percona-toolkit](https://github.com/percona/percona-toolkit)

## License

BSD 3-clause
