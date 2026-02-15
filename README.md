# Cuento Backend

Cuento Backend is a high-performance, feature-rich backend for a roleplaying forum platform, built using Go (Golang) and the Gin web framework. It supports dynamic entity management, real-time notifications, and a structured forum system.

## Features

- **Forum Engine**: Complete support for Categories, Subforums, Topics, and Posts.
- **Rich Text**: BBCode parsing for posts and text fields.
- **Dynamic Entity System**:
  - **Custom Fields**: Entities like Characters and Episodes support dynamic custom fields defined via JSON configuration.
  - **Optimized Storage**: Uses a hybrid approach with EAV (Entity-Attribute-Value) for flexibility and flattened tables (maintained via Triggers) for high-performance querying and sorting.
- **Character Management**:
  - Faction trees with depth-first search ordering.
  - Character profiles with custom avatars and fields.
- **Real-Time Features**:
  - WebSocket hub for live notifications.
  - In-memory Event Bus to decouple logic (e.g., stats updates, async notifications).
- **Authentication**: JWT-based middleware.

## Tech Stack

- **Language**: Go
- **Framework**: [Gin](https://github.com/gin-gonic/gin)
- **Database**: MySQL / MariaDB
- **WebSockets**: [Gorilla WebSocket](https://github.com/gorilla/websocket)
- **BBCode**: [Frustra BBCode](https://github.com/frustra/bbcode)

## Getting Started

### Prerequisites

- Go 1.18+
- MySQL or MariaDB database

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/cuento-backend.git
   cd cuento-backend
   ```

2. **Install Dependencies**
   ```bash
   go mod tidy
   ```

3. **Database Setup**
   Ensure your database is running. The application expects a database connection (configure in `src/Services/db.go`).
   
   To install the initial schema:
   ```bash
   # Start the server
   go run main.go
   
   # Visit the install endpoint (dev only)
   curl http://localhost:8080/install
   ```

4. **Run the Server**
   ```bash
   go run main.go
   ```
   The server runs on port `8080` by default.

## API Endpoints

### Public
- `GET /ping` - Health check.
- `POST /register` - Register a new user.
- `POST /login` - User login.
- `GET /board/info` - Get board statistics (users, posts, etc.).
- `GET /categories/home` - Get forum structure.
- `GET /viewforum/:subforum/:page` - List topics in a subforum.
- `GET /viewtopic/:id/:page` - List posts in a topic.
- `GET /character-list` - Get all active characters grouped by faction.

### Protected (Bearer Token)
- **Characters**
  - `GET /character/get/:id` - Get character details.
  - `POST /character/create` - Create a new character.
  - `PATCH /character/update/:id` - Update character fields.
- **Templates (Custom Fields)**
  - `GET /template/:type/get` - Get field config for an entity type (e.g., 'character', 'episode').
  - `POST /template/:type/update` - Update field config and regenerate database tables.
- **Episodes**
  - `POST /episode/create` - Create a new roleplay episode.
- **Factions**
  - `GET /faction-children/get` - Get faction hierarchy.
- **WebSockets**
  - `GET /ws` - Connect to the WebSocket hub.

## Architecture

### Custom Field System
The system allows admins to define custom fields for entities on the fly.
1. **Configuration**: Stored as JSON in `custom_field_config`.
2. **Data Storage**:
   - `_main` table: Stores data in a vertical format (Entity ID, Field Name, Value).
   - `_flattened` table: A standard table where columns match the field names.
3. **Synchronization**: Database triggers automatically update the flattened table whenever the main table changes, ensuring fast read speeds for filtering and sorting.

### Event Bus
The application uses an internal `EventBus` to handle side effects. For example, when a `TopicCreated` event occurs:
- A subscriber updates the global post/topic counts.
- A subscriber updates the specific subforum stats.
- A subscriber pushes a notification to the WebSocket hub.