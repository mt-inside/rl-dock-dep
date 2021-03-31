foo=$(http :8080/deployments name=foo image=nginx replicas:=3 | jq -r '.deployment.id')
bar=$(http :8080/deployments name=bar image=wordpress replicas:=2 | jq -r '.deployment.id')

echo made $foo
echo made $bar

sleep 5

http :8080/deployments
docker ps

sleep 5

http PATCH :8080/deployment/${foo} name=foo image=nginx replicas:=5

sleep 5

docker ps

sleep 5

http DELETE :8080/deployment/${foo}

sleep 5

docker ps

sleep 5

http DELETE :8080/deployment/${bar}

sleep 5

docker ps
