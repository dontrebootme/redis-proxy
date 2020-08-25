test: unit end-to-end run

clean:
	docker-compose down

unit:
	docker-compose up unit-tests

end-to-end:
	docker-compose up -d redis
	docker-compose up --build end-to-end

run:
	docker-compose up -d redis
	docker-compose up -d proxy
	@echo "Proxy has been started inside redis-proxy_proxy_1"
