#!/bin/sh
set -eu

wait_host() {
  host="$1"
  i=0
  echo "Waiting for ${host}..."
  until mongosh --host "${host}:27017" --quiet --eval "db.runCommand({ping:1}).ok" | grep -q 1; do
    i=$((i + 1))
    if [ "$i" -gt 60 ]; then
      echo "timeout waiting for ${host}"
      exit 1
    fi
    sleep 1
  done
}

wait_host mongodb
wait_host mongodb-2
wait_host mongodb-3

echo "Initializing replica set..."
mongosh --host mongodb:27017 --quiet --eval "
try {
  rs.status();
  print('Replica set already initialized');
} catch (e) {
  rs.initiate({
    _id: 'rs0',
    members: [
      {_id: 0, host: 'mongodb:27017'},
      {_id: 1, host: 'mongodb-2:27017'},
      {_id: 2, host: 'mongodb-3:27017'}
    ]
  });
  print('Replica set initiated');
}
"

echo "Waiting for PRIMARY..."
until mongosh --host mongodb:27017 --quiet --eval "rs.isMaster().ismaster" | grep -q true; do
  sleep 1
done

echo "MongoDB replica set is ready"
