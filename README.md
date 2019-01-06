# dg-hibp
Checks emails in messages to Dr.Gero against HIBP API. Results (if any) are sent to Dr.Gero with a `hibpwner` User-Agent for identification by other consumers.

# Config
dg-hibp requires two environment variables to be set in order to run.

`DG_CONNECT` - A connect string for a RabbitMQ server. Example: `DG_CONNECT=amqp://guest:guest@localhost:5672`.

`DG_HOST` - A host running the Dr.Gero API. Example `DG_HOST=http://localhost:8080`.
