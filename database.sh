
# https://www.elastic.co/guide/en/elasticsearch/reference/current/docker.html
docker run -d -p 9200:9200 -p 9300:9300 -e "discovery.type=single-node" docker.elastic.co/elasticsearch/elasticsearch:7.10.0

# https://hub.docker.com/_/influxdb/
docker run -d -p 8086:8086 \
      -v influxdb:/var/lib/influxdb \
      influxdb

# https://grafana.com/docs/grafana/latest/installation/docker/
docker run -d -p 3000:3000 --name grafana grafana/grafana:6.5.0

# https://www.elastic.co/guide/en/kibana/current/docker.html
DOCKER_REPO=docker.elastic.co/kibana/kibana
DOCKER_VERSION=7.10.2
docker run -d --link $(docker ps| grep elasticsearch | awk '{print $1}'):elasticsearch -p 5601:5601 $DOCKER_REPO:$DOCKER_VERSION
