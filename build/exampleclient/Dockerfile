# Copyright (c) 2009 & onwards. MapR Tech, Inc., All rights reserved

FROM alpine:latest
RUN apk add --no-cache bash
# Modify environment for containers
COPY start.sh /start.sh
RUN chmod +x /start.sh
CMD ["bash -c /start.sh"]
