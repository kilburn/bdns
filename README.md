BDNS, a backup dns zone manager
---------------------------------

BDNS is a simple daemon that exposes an HTTP interface for your primary DNS servers to remotely add/remove zones to/from a secondary server.

The DNS's AXFR system for replication is a bit tricky to set up, but once done it works very reliably. The only problem is that of initially creating/deleting the zones on the secondary servers. This is where BDNS comes in, solving (part of) the problem in three simple steps:

1. Install bdns on the secondary server
2. Setup your primary server so that it allows transfers and notifies your secondary (or secondaries).
3. Setup some system (cron, icron, control panel triggered script, whatever) on the primary server that adds/removes zones through the HTTP interface exposed by bdns. An example of that is the [bdns-client](https://github.com/kilburn/bdns-client) script.

Installation
============

In the near future we plan to release binaries for the common platforms, but for now you have to compile BDNS for your architecture first. The advantage of go binaries is that they don't have any dependency, so you can already run that on your server. Check the program's available options:

```
# ./bdns -h
Usage of ./bdns:
  -configfile string
    	Set the configuration file to use (default "/etc/bdns/bdns.conf")
  -dumpconfig
    	Dumps the effective configuration settings and terminates
  -path string
    	Path to bind's data directory (default "/var/cache/bind")
  -port int
    	Port where to listen (default 54515)
  -rndc string
    	Path to the rndc executable (default "/usr/sbin/rndc")
  -ssl_cert
    	Path to the certificate (bundle)
  -ssl_enabled
    	Enables https
  -ssl_key
    	Path to the certificate key
  -syslog
    	Send logs to syslog
  -zonefile string
    	Set the bind's zone file to read (default "3bf305731dd26307.nzf")
```

All those settings can also be setup through the configuration file. We recommend you to copy the `doc/bdns.conf` to your system's `/etc/bdns/bdns.conf` (the default location where bdns will look for unless you override it throught the command line). Adapt the configuration to fit your system and your requirements.

Finally, you need some way to automatically startup the daemon on every system start. We provide an init script `doc/initscript.sh` that uses lsb init standards, and can be used for that purpose. A `systemd` service file would be more than welcome as a pull request through.

The bdns interface
==================

Bdns exposes an HTTP text/plain interface, and hence it is really easy to test using `curl` or similar utilites. You just need to make sure to use http authentication with a username and password that has been defined in your bdns's configuration file.

These are the methods you can call and their description:

| Path                 | Description 
-----------------------|--------------
| `/list`              | Returns a list of the zone names (e.g.: domain.tld) for which you are currently a master.
| `/add/<zone.tld>`    | Adds `<zone.tld>`, with the requests' originating IP as its master.
| `/remove/<zone.tld>` | Removes `<zone.tld>` from the list of zones mastered by the requests' originating IP.
| `/`                  | Lists all master IPs and their currently associated zones.

Notice that the master is always automatically assumed to be the IP from which the requests are originating. Hence, you need to make sure that you use the proper interface when sending requests to bdns.

Also, notice that unlike most systems which use `json` or similar encodings, bdns uses plain test responses.

### Examples

Assume we start with a fresh `bdns` instance, on a fresh server where no zones have ever been setup. 

First, we list all zones and their masters, getting an empty `200` (ok) response because there are no zones setup yet:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/
HTTP/1.1 200 OK
Content-Length: 0
Content-Type: text/plain
Date: Tue, 07 Jun 2016 10:25:24 GMT
```

Next, we list the zones belonging to the master that initiates the request (127.0.0.1), which obviously gives us another empty list:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/list/
HTTP/1.1 200 OK
Content-Length: 0
Content-Type: text/plain
Date: Tue, 07 Jun 2016 10:25:26 GMT
```

Now we add a domain:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/add/domain.tld
HTTP/1.1 200 OK
Content-Length: 2
Content-Type: text/plain
Date: Tue, 07 Jun 2016 10:25:34 GMT

OK
```

And check that it has been added with the origin IP (127.0.0.1) as its master:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/list/
HTTP/1.1 200 OK
Content-Length: 11
Content-Type: text/plain
Date: Tue, 07 Jun 2016 10:25:36 GMT

domain.tld
```

Hence, listing all zones with their associated master should also show that domain:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/
HTTP/1.1 200 OK
Content-Length: 21
Content-Type: text/plain
Date: Tue, 07 Jun 2016 10:25:40 GMT

domain.tld	127.0.0.1
```

At this point, we cannot add `domain.tld` again (neither to us nor to any other master):
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/add/domain.tld
HTTP/1.1 500 Internal Server Error
Content-Length: 84
Content-Type: text/plain; charset=utf-8
Date: Tue, 07 Jun 2016 10:25:46 GMT
X-Content-Type-Options: nosniff

Error adding zone domain.tld (Zone "domain.tld" is already assigned to "127.0.0.1")
```

But we can remove it, of course:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/remove/domain.tld
HTTP/1.1 200 OK
Content-Length: 2
Content-Type: text/plain
Date: Tue, 07 Jun 2016 10:26:04 GMT

OK
```

Though we cannot remove it again, because it is already gone:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/remove/domain.tld
HTTP/1.1 500 Internal Server Error
Content-Length: 55
Content-Type: text/plain; charset=utf-8
Date: Tue, 07 Jun 2016 10:30:24 GMT
X-Content-Type-Options: nosniff

Error removing zone domain.tld (not found in zone map)
```

As we can see by listing all zones again:
```
$ http --verify=no --auth client1:password1 GET https://127.0.0.1:54515/
HTTP/1.1 200 OK
Content-Length: 0
Content-Type: text/plain
Date: Tue, 07 Jun 2016 10:26:06 GMT
```

