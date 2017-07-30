# devfarm (developing)
<pre>
This program make go source code can be compiled and run at remote linux machine.
You can use client program by go compile syntax.

When you use [client install test-program], your source code is zipped and send to server.
After sent out, client will connect to server using websocket for getting stdout and stderr, sending stdin to remote program.
Server will compile, run, connect stds via websocket.
</pre>
