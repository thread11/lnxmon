# lnxmon

<!-- ### Screenshot -->

<!-- 5,000,000+ points -->

<!-- https://cdn.jsdelivr.net/gh/thread11/lnxmon/doc/lnxmon.png -->

### Support

<!-- - <del>CentOS 5.x (Not Supported)</del> -->
- Arch Linux 64-bit
- CentOS 6.x, 7.x, 8.x 64-bit
- Debian 8.x, 9.x, 10.x, 11.x 64-bit
- Ubuntu 16.x, 18.x, 20.x 64-bit

<!--
### Download

https://cdn.jsdelivr.net/gh/thread11/assets/lnxmon/lnxmonsrv
<br />
https://cdn.jsdelivr.net/gh/thread11/assets/lnxmon/lnxmoncli
-->

### Server

```
# Basic
./lnxmonsrv
./lnxmonsrv --help

# Advanced
./lnxmonsrv --host="127.0.0.1"
./lnxmonsrv --host="0.0.0.0"
./lnxmonsrv --port=1234
./lnxmonsrv --gzip=true
./lnxmonsrv --gzip=false
```

### Client

```
# Basic
./lnxmoncli
./lnxmoncli --help

# Advanced
./lnxmoncli --host="127.0.0.1"
./lnxmoncli --port=1234
./lnxmoncli --project="TEST"
./lnxmoncli --debug=true

# Python
python lnxmoncli.py
```

### Access

```
# HTML
http://127.0.0.1:1234/
http://127.0.0.1:1234/?id=1&mode=0
http://127.0.0.1:1234/?id=1&mode=1
http://127.0.0.1:1234/?id=1&offset=240
http://127.0.0.1:1234/?id=1&offset=240&limit=10
http://127.0.0.1:1234/?id=1&offset=240&limit=-1
http://127.0.0.1:1234/?project=default

# API
http://127.0.0.1:1234/api/get_projects
http://127.0.0.1:1234/api/get_hosts
http://127.0.0.1:1234/api/get_hosts?project=default
http://127.0.0.1:1234/api/get_host?id=1
http://127.0.0.1:1234/api/get_host_metric?id=1
http://127.0.0.1:1234/api/get_host_metric?id=1&offset=240
http://127.0.0.1:1234/api/get_host_metric?id=1&offset=240&limit=10
http://127.0.0.1:1234/api/get_host_metric?id=1&offset=240&limit=-1
```
