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
let currentPostID = null;
let lastIDStack = [null]; // Stack supports back/prev navigation for cursor-based pagination
let currentLastID = null;
let nextLastID = null;
const postsPerPage = 10;

// API Functions
const api = {
    async getPosts(page = 1, limit = postsPerPage) {
        // Kept for possible future use, but the UI uses cursor-based pagination
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

    async createComment(postID, body) {
        const response = await fetch(`${API_BASE}/posts/${postID}/comment`, {
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
        // Inline event handlers are used for simplicity; in a larger app, delegation is preferred
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
                        <button class="view ${commentClass}" onclick="(() => showPostDetail('${post.id}'))()">${commentLabel}</button>
                    </span>
                    <button class="delete" onclick="deletePost('${post.id}')" aria-label="Delete post">
                        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2"></path><line x1="10" y1="11" x2="10" y2="17"></line><line x1="14" y1="11" x2="14" y2="17"></line></svg>
                    </button>
                </div>
            </div>
        `;
        return div;
    },

    updatePostDetail(post) {
        // Always update modal content to ensure fresh data after comment creation
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
        // Modal keeps the user on the same page context
        elements.postDetail.classList.remove('hidden');
        elements.postDetail.style.display = 'block';
        document.body.classList.add('modal-open');
        document.body.style.overflow = 'hidden';
    },

    hideModal() {
        // Reset modal state and update the URL to reflect the post list view
        elements.postDetail.classList.add('hidden');
        document.body.classList.remove('modal-open');
        document.body.style.overflow = '';
        currentPostID = null;
        history.pushState({ type: 'page' }, '', '/posts');
    },

    resetCommentForm() {
        // Reset the comment form after a comment is added or modal is opened
        elements.commentBodyInput.value = '';
        elements.addCommentForm.style.display = 'flex';
    },

    showError(message) {
        // Log errors to the console for debugging; in production, show user-friendly messages
        console.error(message);
    },

    createPaginationControls() {
        // Always show both buttons for consistent UI, but disable them as needed
        const div = document.createElement('div');
        div.className = 'pagination';
        
        // Previous button is disabled on the first page (stack length 1)
        const prevButton = document.createElement('button');
        prevButton.textContent = '←';
        prevButton.className = lastIDStack.length <= 1 ? 'disabled' : '';
        prevButton.onclick = () => {
            if (lastIDStack.length > 1) goToPrevPage();
        };
        
        // Next button is disabled if there are no more posts (see fetchPosts for logic)
        const nextButton = document.createElement('button');
        nextButton.textContent = '→';
        nextButton.className = !nextLastID ? 'disabled' : '';
        nextButton.onclick = () => {
            if (nextLastID) goToNextPage();
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
    if (currentLastID) params.append('last_id', currentLastID);

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

    // Peek ahead to ensure the next button is only enabled if the next page has posts
    if (posts.length === postsPerPage && data.next_last_id) {
        const peekParams = new URLSearchParams();
        peekParams.append('limit', postsPerPage);
        peekParams.append('last_id', data.next_last_id);
        const peekResponse = await fetch(`${API_BASE}/posts?${peekParams.toString()}`);
        const peekData = await peekResponse.json();
        if (Array.isArray(peekData.posts) && peekData.posts.length > 0) {
            nextLastID = data.next_last_id;
        } else {
            nextLastID = null;
        }
    } else {
        nextLastID = null;
    }
    ui.createPaginationControls();
    updateUrlForPage();
}

async function showPostDetail(id) {
    try {
        const post = await api.getPostDetail(id);
        currentPostID = id;
        ui.updatePostDetail(post);
        ui.resetCommentForm();
        ui.showModal();
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
        if (currentPostID === id) {
            ui.hideModal();
        }
    } catch (error) {
        ui.showError('Failed to delete post');
    }
}

// Event Listeners
// Use event delegation for modal close to allow clicking outside the modal to close it
// but not when clicking inside or on the view button
// This improves UX by making the modal easy to dismiss
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
    if (!body || !currentPostID) return;
    try {
        await api.createComment(currentPostID, body);
        elements.commentBodyInput.value = '';
        await showPostDetail(currentPostID);
        await fetchPosts();
    } catch (error) {
        ui.showError('Failed to add comment');
    }
});

// Expose deletePost globally for use in inline event handlers
window.deletePost = deletePost;

// Pagination controls
function goToNextPage() {
    // Block navigation if there is no next page, to prevent empty page views
    if (!nextLastID) return;
    lastIDStack.push(nextLastID);
    currentLastID = nextLastID;
    fetchPosts();
}

function goToPrevPage() {
    // Only allow going back if there is a previous page in the stack
    if (lastIDStack.length > 1) {
        lastIDStack.pop();
        currentLastID = lastIDStack[lastIDStack.length - 1];
        fetchPosts();
    }
}

function updateUrlForPage() {
    // Always update the URL to reflect the current pagination state for shareability and navigation
    const params = new URLSearchParams();
    params.append('limit', postsPerPage);
    if (currentLastID) params.append('last_id', currentLastID);
    history.pushState({ type: 'page', lastID: currentLastID }, '', `?${params.toString()}`);
}

function updateUrlForPostDetail(postID) {
    // Use pushState to allow browser navigation and deep linking to post details
    history.pushState({ type: 'post', postID }, '', `/posts/${postID}`);
}

// Helper: Find and load the page containing a specific post ID
async function loadPageContainingPost(postID) {
    // Find the correct page for a post so the background list matches the detail
    let lastID = null;
    let found = false;
    let pagePosts = [];
    let pageStack = [null];
    while (!found) {
        const params = new URLSearchParams();
        params.append('limit', postsPerPage);
        if (lastID) params.append('last_id', lastID);
        const response = await fetch(`${API_BASE}/posts?${params.toString()}`);
        const data = await response.json();
        if (!Array.isArray(data.posts) || data.posts.length === 0) break;
        pagePosts = data.posts;
        if (pagePosts.some(post => post.id === postID)) {
            found = true;
            break;
        }
        if (!data.next_last_id) break;
        lastID = data.next_last_id;
        pageStack.push(lastID);
    }
    if (found) {
        lastIDStack = [...pageStack];
        currentLastID = lastIDStack[lastIDStack.length - 1];
    } else {
        lastIDStack = [null];
        currentLastID = null;
    }
}

function restoreFromUrl() {
    // Handle all routing here to support deep links, browser navigation, and SPA behavior
    const path = window.location.pathname;
    const search = window.location.search;
    if (path === '/' || path === '') {
        // Always redirect / to /posts for consistency and shareability
        history.replaceState({ type: 'page' }, '', '/posts');
        return;
    }
    if (path.startsWith('/posts/') && /^\/posts\/\d+$/.test(path)) {
        // When accessing a post detail, ensure the background list is the correct page
        const postID = path.split('/')[2];
        loadPageContainingPost(postID).then(() => {
            fetchPosts().then(() => showPostDetail(postID));
        });
    } else {
        // For pagination, restore the correct page from the URL
        const params = new URLSearchParams(search);
        const lastID = params.get('last_id');
        currentLastID = lastID;
        lastIDStack = [null];
        if (lastID) lastIDStack.push(lastID);
        fetchPosts();
    }
}

window.onpopstate = function(event) {
    // Listen for browser navigation to keep UI and URL in sync
    restoreFromUrl();
};

// On initial load, restore state from the URL for deep linking and refresh support
restoreFromUrl(); 
