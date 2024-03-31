# Cproxy

A Container Proxy for both Windows, Linux and macOS.

## Usage

1. Firstly, you sould got a domain with wildcard support.
1. Then , just add a wildcard `A` record of your domain to your server.
1. Finally, running *cproxy* and just enjoy!

**Which IP should the domain resolve to?**

- If you'd like to use it on your **local** *dev server* only, just set it to `127.0.0.1`.

- If you'd like to use it on your **LAN**, set it to LAN IP of your server, such as `192.168.1.3`.

- If you'd like to use it **all over the world**, set it to WAN IP of your server, such as `8.8.8.8`.

Now, you can visit you service by `http://containername.mydebug.com`.

All your request will auto forward into your container.
