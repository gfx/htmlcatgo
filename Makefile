
logcat:
	tail -f  ~/Library/Logs/iOS\ Simulator/*/system.log | go run ios-simulator-logcat.go
