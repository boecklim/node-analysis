#!/bin/bash

# Set session name
SESSION_NAME="my_panes_session"

# Create a new tmux session (detached)
tmux new-session -d -s $SESSION_NAME

# Split the window into panes
tmux send-keys "./connect.sh 0" C-m  # Command for the first (default) pane

tmux split-window -h                  # Create the second pane
tmux send-keys "./connect.sh 1" C-m

tmux split-window -h                  # Create the third pane
tmux send-keys "./connect.sh 2" C-m

tmux split-window -h                  # Create the fourth pane
tmux send-keys "./connect.sh 3" C-m

tmux split-window -h                  # Create the fifth pane
tmux send-keys "./connect.sh 4" C-m

# Balance panes to ensure equal size
tmux select-layout tiled

# Attach to the tmux session
tmux attach-session -t $SESSION_NAME
