#!/bin/sh
set -eu

echo "Waiting for MongoDB nodes..."
sleep 5

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
