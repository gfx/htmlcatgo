
logcat:
	tail -f  ~/Library/Logs/iOS\ Simulator/7.0.3-64/system.log | go run htmlcat.go
