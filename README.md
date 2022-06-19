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

## References

* [percona/go-mysql](https://github.com/percona/go-mysql) Go packages for MySQL
* [pingcap/parser](https://github.com/pingcap/parser) A MySQL Compatible SQL Parser
* [percona/percona-toolkit](https://github.com/percona/percona-toolkit)

## License

BSD 3-clause
