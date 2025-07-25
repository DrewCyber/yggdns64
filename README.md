# DNS64 for yggdrasil network

A simple DNS64 proxy written in go based on [github.com/miekg/dns](https://github.com/miekg/dns)

Unlike 'regular' DNS64 servers, it does not return a 'white' IPv6 address even if one exists. However, if there is an AAAA record with the yggdrasil address, it returns that specifically.

## How to configure zones block
 
Standard case. Nat64 + do not return a 'white' IPV6 address even if one exists:
```
zones:
  default:
    domains:                        # Zone domains filter
      - "."
    prefix: "300:dada:feda:f123:ff::" # Convert A records to AAAA
    return-public-ipv4: false       # Do not return 'white' A records
```
Route *.example.com and *.com.tr domains directly with ipv4, all other traffic forward to Nat64:
```
zones:
  my-direct-zone:
    domains:                        # Zone domains filter
      - "example.com"
      - "com.tr"
    return-public-ipv4: true        # Return 'white' A records
  default:
    domains:                        # Zone domains filter
      - "."
    prefix: "300:dada:feda:f123:ff::" # If prefix is set, then it will convert A records to AAAA
    return-public-ipv4: false       # Do not return 'white' A records
```
Opposite case. By default route all with ipv4, but forward *.example.com and *.com.tr domains to Nat64:
```
zones:
  my-direct-zone:
    domains:                        # Zone domains filter
      - "example.com"
      - "com.tr"
    return-public-ipv4: false        # Return 'white' A records
    prefix: "300:dada:feda:f123:ff::" # If prefix is set, then it will convert A records to AAAA
  default:
    domains:                        # Zone domains filter
      - "."
    return-public-ipv4: true       # Return 'white' A records
```
You have routable ipv4 and 2 nat64 servers:
```
zones:
  zone1:
    domains:                        # Zone domains filter
      - "com.tr"
    return-public-ipv4: true        # Return 'white' A records
  zone2:
    domains:                        # Zone domains filter
      - "com"
    prefix: "300:baba:feda:f123:ff::"
  default:
    domains:                        # Zone domains filter
      - "."
    prefix: "300:dada:feda:f123:ff::" # If prefix is set, then it will convert A records to AAAA
    return-public-ipv4: false       # Do not return 'white' A records
```
No wildcard zone. Resolve only particular domains. For all other request return NXDOMAIN
```
zones:
  default:
    domains:                        # Zone domains filter
      - "example.com"
      - "com.tr"
    prefix: "300:dada:feda:f123:ff::" # If prefix is set, then it will convert A records to AAAA
    return-public-ipv4: false       # Do not return 'white' A records
```


## Build
`go build .`
## Run
`./yggdns64 -file ./config.yml`
## Test
```
dig ya.ru any  @127.0.0.1 -p 1053
dig ya.ru AAAA  @127.0.0.1 -p 1053
dig ya.ru A  @127.0.0.1 -p 1053
```
## Create systemd service
```
cp ./yggdns64.service /etc/systemd/system/yggdns64.service
systemctl daemon-reload
systemctl start yggdns64
systemctl enable yggdns64
```

### TODO:  
- [x] zones config
- [x] general domains list handling
- [x] 'strict-ipv6: yes' replace with 'return-public-ipv4: no'
- [x] convert-a-to-aaaa if prefix is set (prefix: "300:dada:feda:f443:ff::")
- [ ] use domains-file to import domains list
- [ ] return-public-ipv6: true
- [ ] check domains config regexp "example.com" and "." presence.