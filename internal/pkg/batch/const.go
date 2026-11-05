package batch

const (
	workerPrefix            string = "[Worker] "
	ChannelConcurrentWorker string = "ChannelConcurrentWorker"
	// containers/image wraps a 404 on delete in this sentence, there is no typed error for it.
	absentOnDelete string = "Image may not exist or is not stored with a v2 Schema"
)
