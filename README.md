# playdot - A simple playground for dpic and dot

A simple webservice that displays an editor for dpic- and dot-input and shows
the SVG-output or error messages below it.  Allows for sharing of links to the
input.

You must first install 
    - dpic from https://ece.uwaterloo.ca/~aplevich/dpic and 
    - graphviz from https://graphviz.org

before you can use playdot properly.

## Install

	% go get github.com/oec/playdot
	% cd $GOPATH/src/github.com/oec/playdot
	% go build

To run the server on port 9999 without TLS:

	% ./playdot -l :9999
	2017/11/22 18:13:13 handler for /dot/ registered
	2017/11/22 18:13:13 handler for /dpic/ registered
	2017/11/22 18:13:13 listening non-tls on :9999

See `./playdot -h` for further options.
