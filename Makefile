all: chat

chat:
	@go build -o chat

clean:
	@rm chat

test:
	@docker network create lan-net

	@docker run -d --name node1 --network lan-net -v ./:/app ubuntu:latest sleep infinity
	@docker run -d --name node2 --network lan-net -v ./:/app ubuntu:latest sleep infinity
	@docker run -d --name node3 --network lan-net -v ./:/app ubuntu:latest sleep infinity

	@tmux new-session -d -s lan-discovery
	@tmux split-window -h
	@tmux split-window -v

	@tmux select-pane -t 0
	@tmux send-keys "docker exec -it node1 bash" C-m
	@tmux send-keys "cd /app && ./chat" C-m

	@tmux select-pane -t 1
	@tmux send-keys "docker exec -it node2 bash" C-m
	@tmux send-keys "cd /app && ./chat" C-m

	@tmux select-pane -t 2
	@tmux send-keys "docker exec -it node3 bash" C-m
	@tmux send-keys "cd /app && ./chat" C-m

	@tmux attach -t lan-discovery
