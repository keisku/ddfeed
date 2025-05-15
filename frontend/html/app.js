// Constants
const API_BASE = 'http://localhost:16080/ui/v1';

// DOM Elements
const elements = {
    postsList: document.getElementById('posts-list'),
    addPostForm: document.getElementById('add-post-form'),
    postBodyInput: document.getElementById('post-body'),
    postDetail: document.getElementById('post-detail'),
    postContent: document.getElementById('post-content'),
    commentsList: document.getElementById('comments-list'),
    addCommentForm: document.getElementById('add-comment-form'),
    commentBodyInput: document.getElementById('comment-body'),
    pagination: document.getElementById('pagination')
};

// State
let currentPostId = null;
let lastIdStack = [null]; // null for the first page
let currentLastId = null;
let nextLastId = null;
const postsPerPage = 10;

// API Functions
const api = {
    async getPosts(page = 1, limit = postsPerPage) {
        const response = await fetch(`${API_BASE}/posts?page=${page}&limit=${limit}`);
        if (!response.ok) throw new Error('Failed to fetch posts');
        return response.json();
    },

    async getPostDetail(id) {
        const response = await fetch(`${API_BASE}/posts/${id}`);
        if (!response.ok) throw new Error('Failed to fetch post details');
        return response.json();
    },

    async createPost(body) {
        const response = await fetch(`${API_BASE}/posts`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ body })
        });
        if (!response.ok) throw new Error('Failed to create post');
        return response.json();
    },

    async createComment(postId, body) {
        const response = await fetch(`${API_BASE}/posts/${postId}/comment`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ body })
        });
        if (!response.ok) throw new Error('Failed to create comment');
        return response.json();
    },

    async deletePost(id) {
        const response = await fetch(`${API_BASE}/posts/${id}`, { method: 'DELETE' });
        if (!response.ok) throw new Error('Failed to delete post');
    }
};

// UI Functions
const ui = {
    createPostElement(post) {
        const div = document.createElement('div');
        div.className = 'post-item';
        const commentCount = post.comment_count || 0;
        let commentLabel = '';
        let commentClass = '';
        if (commentCount === 0) {
            commentLabel = 'Comment';
            commentClass = 'no-comments';
        } else if (commentCount === 1) {
            commentLabel = '1 comment';
            commentClass = 'has-comments';
        } else {
            commentLabel = `${commentCount} comments`;
            commentClass = 'has-comments';
        }
        div.innerHTML = `
            <div class="post-content">
                <div class="post-body-text">${post.body}</div>
                <div class="post-meta">
                    <span class="post-actions">
                        <button class="view ${commentClass}" onclick="(() => showPostDetail(${post.id}))()">${commentLabel}</button>
                        <button class="delete" onclick="deletePost(${post.id})">Delete</button>
                    </span>
                </div>
            </div>
        `;
        return div;
    },

    updatePostDetail(post) {
        elements.postContent.innerHTML = `
            <div class="post-detail-content">
                <p id="post-body-text">${post.body}</p>
            </div>
        `;

        elements.commentsList.innerHTML = '';
        if (post.comments?.length > 0) {
            post.comments.forEach(comment => {
                const li = document.createElement('li');
                li.className = 'comment-item';
                li.innerHTML = `<p>${comment.body}</p>`;
                elements.commentsList.appendChild(li);
            });
        }
    },

    showModal() {
        elements.postDetail.classList.remove('hidden');
        elements.postDetail.style.display = 'block';
        document.body.classList.add('modal-open');
        document.body.style.overflow = 'hidden';
    },

    hideModal() {
        elements.postDetail.classList.add('hidden');
        document.body.classList.remove('modal-open');
        document.body.style.overflow = '';
        currentPostId = null;
        history.pushState({ type: 'page' }, '', '/posts');
    },

    resetCommentForm() {
        elements.commentBodyInput.value = '';
        elements.addCommentForm.style.display = 'flex';
    },

    showError(message) {
        console.error(message);
    },

    createPaginationControls() {
        const div = document.createElement('div');
        div.className = 'pagination';
        
        // Previous button
        const prevButton = document.createElement('button');
        prevButton.textContent = '←';
        prevButton.className = lastIdStack.length <= 1 ? 'disabled' : '';
        prevButton.onclick = () => {
            if (lastIdStack.length > 1) goToPrevPage();
        };
        
        // Next button
        const nextButton = document.createElement('button');
        nextButton.textContent = '→';
        nextButton.className = !nextLastId ? 'disabled' : '';
        nextButton.onclick = () => {
            if (nextLastId) goToNextPage();
        };
        
        div.appendChild(prevButton);
        div.appendChild(nextButton);
        
        elements.pagination.innerHTML = '';
        elements.pagination.appendChild(div);
    }
};

// Event Handlers
async function fetchPosts() {
    const params = new URLSearchParams();
    params.append('limit', postsPerPage);
    if (currentLastId) params.append('last_id', currentLastId);

    const response = await fetch(`${API_BASE}/posts?${params.toString()}`);
    const data = await response.json();

    elements.postsList.innerHTML = '';
    
    let posts = [];
    if (Array.isArray(data.posts)) {
        posts = data.posts;
        data.posts.forEach(post => {
            elements.postsList.appendChild(ui.createPostElement(post));
        });
    }

    // Peek ahead if needed
    if (posts.length === postsPerPage && data.next_last_id) {
        // Peek the next page
        const peekParams = new URLSearchParams();
        peekParams.append('limit', postsPerPage);
        peekParams.append('last_id', data.next_last_id);
        const peekResponse = await fetch(`${API_BASE}/posts?${peekParams.toString()}`);
        const peekData = await peekResponse.json();
        if (Array.isArray(peekData.posts) && peekData.posts.length > 0) {
            nextLastId = data.next_last_id;
        } else {
            nextLastId = null;
        }
    } else {
        nextLastId = null;
    }
    ui.createPaginationControls();
    updateUrlForPage();
}

async function showPostDetail(id) {
    try {
        const post = await api.getPostDetail(id);
        currentPostId = id;
        ui.updatePostDetail(post);
        ui.resetCommentForm();
        ui.showModal();
        // Only update the URL if it's not already /posts/:id
        const expectedPath = `/posts/${id}`;
        if (window.location.pathname !== expectedPath) {
            updateUrlForPostDetail(id);
        }
    } catch (error) {
        ui.showError('Failed to load post details');
    }
}

async function deletePost(id) {
    if (!confirm('Delete this post?')) return;
    try {
        await api.deletePost(id);
        await fetchPosts();
        if (currentPostId === id) {
            ui.hideModal();
        }
    } catch (error) {
        ui.showError('Failed to delete post');
    }
}

// Event Listeners
document.addEventListener('click', (event) => {
    if (!elements.postDetail.classList.contains('hidden') && 
        !elements.postDetail.contains(event.target) && 
        !event.target.classList.contains('view')) {
        ui.hideModal();
    }
});

elements.postDetail.addEventListener('click', (event) => {
    event.stopPropagation();
});

elements.addPostForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const body = elements.postBodyInput.value.trim();
    if (!body) return;
    
    try {
        await api.createPost(body);
        elements.postBodyInput.value = '';
        await fetchPosts();
    } catch (error) {
        ui.showError('Failed to create post');
    }
});

elements.addCommentForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const body = elements.commentBodyInput.value.trim();
    if (!body || !currentPostId) return;
    
    try {
        await api.createComment(currentPostId, body);
        elements.commentBodyInput.value = '';
        await showPostDetail(currentPostId);
        await fetchPosts();
    } catch (error) {
        ui.showError('Failed to add comment');
    }
});

// Make deletePost available globally
window.deletePost = deletePost;

// Initialize
fetchPosts();

// Pagination controls
function goToNextPage() {
    // Block navigation if there is no next page
    if (!nextLastId) return;
    lastIdStack.push(nextLastId);
    currentLastId = nextLastId;
    fetchPosts();
}

function goToPrevPage() {
    if (lastIdStack.length > 1) {
        lastIdStack.pop();
        currentLastId = lastIdStack[lastIdStack.length - 1];
        fetchPosts();
    }
}


function updateUrlForPage() {
    const params = new URLSearchParams();
    params.append('limit', postsPerPage);
    if (currentLastId) params.append('last_id', currentLastId);
    history.pushState({ type: 'page', lastId: currentLastId }, '', `?${params.toString()}`);
}

function updateUrlForPostDetail(postId) {
    history.pushState({ type: 'post', postId }, '', `/posts/${postId}`);
}

// Helper: Find and load the page containing a specific post ID
async function loadPageContainingPost(postId) {
    let lastId = null;
    let found = false;
    let pagePosts = [];
    let pageStack = [null];
    while (!found) {
        const params = new URLSearchParams();
        params.append('limit', postsPerPage);
        if (lastId) params.append('last_id', lastId);
        const response = await fetch(`${API_BASE}/posts?${params.toString()}`);
        const data = await response.json();
        if (!Array.isArray(data.posts) || data.posts.length === 0) break;
        pagePosts = data.posts;
        if (pagePosts.some(post => post.id === postId)) {
            found = true;
            break;
        }
        if (!data.next_last_id) break;
        lastId = data.next_last_id;
        pageStack.push(lastId);
    }
    if (found) {
        lastIdStack = [...pageStack];
        currentLastId = lastIdStack[lastIdStack.length - 1];
    } else {
        lastIdStack = [null];
        currentLastId = null;
    }
}

function restoreFromUrl() {
    const path = window.location.pathname;
    const search = window.location.search;
    if (path === '/' || path === '') {
        history.replaceState({ type: 'page' }, '', '/posts');
        return;
    }
    if (path.startsWith('/posts/') && /^\/posts\/\d+$/.test(path)) {
        // Post detail
        const postId = parseInt(path.split('/')[2], 10);
        loadPageContainingPost(postId).then(() => {
            fetchPosts().then(() => showPostDetail(postId));
        });
    } else {
        // Pagination
        const params = new URLSearchParams(search);
        const lastId = params.get('last_id');
        currentLastId = lastId;
        // Rebuild lastIdStack for back/forward navigation
        lastIdStack = [null];
        if (lastId) lastIdStack.push(lastId);
        fetchPosts();
    }
}

window.onpopstate = function(event) {
    restoreFromUrl();
};

restoreFromUrl(); 
